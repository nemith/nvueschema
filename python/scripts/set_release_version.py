"""Stamp both package versions and their dependency pin from a release tag."""

import argparse
import re
from pathlib import Path

WORKSPACE = Path(__file__).resolve().parents[1]
NUMBER = r"(?:0|[1-9][0-9]*)"
TAG = re.compile(rf"v({NUMBER}\.{NUMBER}\.{NUMBER})(?:-(alpha|beta|rc)\.({NUMBER}))?")


def release_version(tag):
    match = TAG.fullmatch(tag)
    if not match:
        raise ValueError("Expected a tag like v0.1.0 or v0.2.0-rc.1")
    version, stage, number = match.groups()
    if stage:
        version += {"alpha": "a", "beta": "b", "rc": "rc"}[stage] + number
    return version


def stamp(workspace, tag):
    version = release_version(tag)
    updates = {}
    for package in ("nvueschema", "nvueschema-binary"):
        path = workspace / package / "pyproject.toml"
        original = path.read_text()
        updated, count = re.subn(
            r'^version = "[^"]+"$',
            f'version = "{version}"',
            original,
            flags=re.MULTILINE,
        )
        if count != 1:
            raise ValueError(f"Expected one package version in {path}")
        if package == "nvueschema":
            updated, count = re.subn(
                r'"nvueschema-binary==[^"\s]+"',
                f'"nvueschema-binary=={version}"',
                updated,
            )
            if count < 1:
                raise ValueError(f"Missing binary dependency pin in {path}")
        updates[path] = updated
    for path, updated in updates.items():
        path.write_text(updated)
    return version


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag", help="GitHub release tag, e.g. v0.1.0")
    args = parser.parse_args()
    try:
        version = stamp(WORKSPACE, args.tag)
    except ValueError as error:
        parser.error(str(error))
    print(f"Release {args.tag}: both Python packages use {version}")


if __name__ == "__main__":
    main()
