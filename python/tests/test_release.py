import runpy
from pathlib import Path

import pytest

WORKSPACE = Path(__file__).resolve().parents[1]
RELEASE = runpy.run_path(str(WORKSPACE / "scripts/set_release_version.py"))


@pytest.mark.parametrize(
    ("tag", "expected"),
    [
        ("v1.2.3", "1.2.3"),
        ("v0.2.0-alpha.1", "0.2.0a1"),
        ("v0.2.0-beta.2", "0.2.0b2"),
        ("v0.2.0-rc.1", "0.2.0rc1"),
    ],
)
def test_stamp_release(tmp_path, tag, expected):
    for package in ("nvueschema", "nvueschema-binary"):
        directory = tmp_path / package
        directory.mkdir()
        (directory / "pyproject.toml").write_text(
            (WORKSPACE / package / "pyproject.toml").read_text()
        )
    assert RELEASE["stamp"](tmp_path, tag) == expected
    for package in ("nvueschema", "nvueschema-binary"):
        text = (tmp_path / package / "pyproject.toml").read_text()
        assert f'version = "{expected}"' in text
    base = (tmp_path / "nvueschema/pyproject.toml").read_text()
    assert f'cli = ["nvueschema-binary=={expected}"]' in base
    assert f'full = ["nvueschema-binary=={expected}"]' in base
    assert 'dependencies = ["pydantic>=2,<3"]' in base


@pytest.mark.parametrize(
    "tag",
    ["", "main", "1.2.3", "v1.2", "v01.2.3", "v1.2.3+local", "v1.2.3\n"],
)
def test_invalid_release_tag(tag):
    with pytest.raises(ValueError, match="Expected a tag"):
        RELEASE["release_version"](tag)


def test_bad_metadata_leaves_files_unchanged(tmp_path):
    base = tmp_path / "nvueschema/pyproject.toml"
    binary = tmp_path / "nvueschema-binary/pyproject.toml"
    for path in (base, binary):
        path.parent.mkdir()
    original = (WORKSPACE / "nvueschema/pyproject.toml").read_text()
    base.write_text(original)
    binary.write_text('[project]\nname = "nvueschema-binary"\n')
    with pytest.raises(ValueError, match="Expected one package version"):
        RELEASE["stamp"](tmp_path, "v1.2.3")
    assert base.read_text() == original
