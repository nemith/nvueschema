"""Exercise the native command installed by a wheel or editable install."""

import json
import os
import shutil
import subprocess
from importlib.metadata import distribution

import pytest


@pytest.fixture
def cli():
    backend = pytest.importorskip("nvueschema_binary")
    command = shutil.which("nvueschema")
    assert command is not None, "Native CLI was not installed on PATH"
    assert backend.executable().samefile(command)
    files = distribution("nvueschema-binary").files
    assert files is not None
    assert (
        len(
            [entry for entry in files if entry.name in ("nvueschema", "nvueschema.exe")]
        )
        == 1
    )
    # A native executable, not a Python console-script launcher.
    with open(command, "rb") as stream:
        magic = stream.read(4)
    assert magic[:2] == b"MZ" or magic in (
        b"\x7fELF",
        b"\xcf\xfa\xed\xfe",
        b"\xfe\xed\xfa\xcf",
    )
    return command


def test_cli_version_and_help(cli):
    result = subprocess.run(
        [cli, "--version"], check=True, capture_output=True, text=True
    )
    assert (
        result.stdout.strip()
        == f"nvueschema version {distribution('nvueschema-binary').version}"
    )
    result = subprocess.run([cli, "--help"], check=True, capture_output=True, text=True)
    assert "validate" in result.stdout


def test_cli_validation(cli, tmp_path):
    spec = tmp_path / "spec.json"
    spec.write_text(
        json.dumps(
            {
                "x-defs": {
                    "cue-patch-schema-root-root": {
                        "properties": {"mtu": {"type": "integer", "minimum": 552}}
                    }
                }
            }
        )
    )
    config = tmp_path / "config.json"
    for mtu, status, valid in [(1500, 0, True), (-1, 1, False)]:
        config.write_text(json.dumps({"mtu": mtu}))
        result = subprocess.run(
            [cli, "validate", "--json", str(spec), str(config)],
            capture_output=True,
            text=True,
        )
        assert result.returncode == status, result.stderr
        assert json.loads(result.stdout)["results"][0]["valid"] is valid


def test_backend_ignores_path(cli, monkeypatch, tmp_path):
    from nvueschema_binary import executable

    impostor = tmp_path / ("nvueschema.exe" if os.name == "nt" else "nvueschema")
    impostor.write_bytes(b"not the installed backend")
    monkeypatch.setenv("PATH", str(tmp_path))
    assert executable().samefile(cli)
