import json
import subprocess
import sys
from types import SimpleNamespace

import pytest

from nvueschema import BackendError, BackendUnavailableError, ValidationError, Validator


@pytest.fixture
def spec(tmp_path):
    path = tmp_path / "spec.json"
    path.write_text(
        json.dumps(
            {
                "x-defs": {
                    "cue-patch-schema-root-root": {
                        "properties": {
                            "mtu": {"type": "integer", "minimum": 552, "maximum": 9216},
                            "profile": {"type": "string", "pattern": "^(?!none$).*$"},
                            "repeat": {"type": "string", "pattern": r"^([a-z]+)-\1$"},
                        }
                    }
                }
            }
        )
    )
    return path


@pytest.fixture
def native():
    pytest.importorskip("nvueschema_native").executable()


def test_missing_backend(monkeypatch, spec):
    monkeypatch.setitem(sys.modules, "nvueschema_native", None)
    with pytest.raises(BackendUnavailableError, match=r"nvueschema\[full\]"):
        Validator(spec=spec).validate({})


def test_validation(native, spec):
    validator = Validator(spec=spec)
    value = {"mtu": 1500, "profile": "fast", "repeat": "same-same"}
    assert validator.validate(value) is None
    assert value == {"mtu": 1500, "profile": "fast", "repeat": "same-same"}
    for key, bad in (
        ("mtu", -1),
        ("mtu", "1500"),
        ("profile", "none"),
        ("repeat", "same-other"),
    ):
        with pytest.raises(ValidationError) as error:
            validator.validate({key: bad})
        assert any(
            e["path"] == f"/{key}" and e["schema_path"] and e["message"]
            for e in error.value.errors
        )


def test_batch(native, spec):
    validator = Validator(spec=spec)
    results = validator.validate_many(iter([{"mtu": 1500}, {"mtu": -1}, {}]))
    assert [r.valid for r in results] == [True, False, True]
    assert results[1].errors
    assert validator.validate_many([]) == []


def test_backend_failure(native, tmp_path):
    with pytest.raises(BackendError, match="loading spec"):
        Validator(spec=tmp_path / "absent.json").validate({})
    path = tmp_path / "bad.json"
    path.write_text("not json")
    with pytest.raises(BackendError):
        Validator(spec=path).validate({})


def test_inputs(spec):
    for kwargs in (
        {},
        {"version": "5.16", "spec": spec},
        {"version": "other"},
        {"version": "5.16", "timeout": 0},
    ):
        with pytest.raises(ValueError):
            Validator(**kwargs)
    with pytest.raises(ValueError):
        Validator(spec=spec).validate({"mtu": float("nan")})
    with pytest.raises(TypeError):
        Validator(spec=spec).validate(object())


@pytest.fixture
def fake_backend(monkeypatch):
    monkeypatch.setitem(
        sys.modules,
        "nvueschema_native",
        SimpleNamespace(executable=lambda: "/fake/backend"),
    )


def test_timeout(fake_backend, monkeypatch, spec):
    def timeout(*args, **kwargs):
        raise subprocess.TimeoutExpired(args[0], kwargs["timeout"])

    monkeypatch.setattr(subprocess, "run", timeout)
    with pytest.raises(BackendError, match="timed out"):
        Validator(spec=spec, timeout=0.1).validate({})


@pytest.mark.parametrize(
    "stdout,status",
    [
        ("not-json", 0),
        ('{"protocol":2,"results":[]}', 0),
        ('{"protocol":1,"results":[]}', 0),
        ('{"protocol":1,"results":[{"valid":true,"issues":[]}]}', 1),
        ('{"protocol":1,"results":[{"valid":false,"issues":[]}]}', 1),
    ],
)
def test_protocol_failure(fake_backend, monkeypatch, spec, stdout, status):
    monkeypatch.setattr(
        subprocess,
        "run",
        lambda *a, **kw: SimpleNamespace(stdout=stdout, stderr="", returncode=status),
    )
    with pytest.raises(BackendError):
        Validator(spec=spec).validate({})
