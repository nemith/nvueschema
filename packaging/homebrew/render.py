"""Render the Homebrew formula using a checked release source archive."""

import argparse
import hashlib
import re
from pathlib import Path


def render(tag, archive, template, source_url=None):
    if not re.fullmatch(
        r"v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)", tag
    ):
        raise ValueError("Homebrew updates require a stable tag such as v0.1.0")
    url = (
        source_url
        or f"https://github.com/nemith/nvueschema/archive/refs/tags/{tag}.tar.gz"
    )
    if not url.startswith(("https://", "file://")) or any(c in url for c in '\n\r"\\#'):
        raise ValueError("Invalid source archive URL")
    return (
        template.replace("@URL@", url)
        .replace("@VERSION@", tag[1:])
        .replace("@SHA256@", hashlib.sha256(archive.read_bytes()).hexdigest())
    )


def would_downgrade(formula, tag):
    match = re.search(r'^  version "([0-9]+\.[0-9]+\.[0-9]+)"$', formula, re.MULTILINE)
    if not match:
        raise ValueError("Existing formula is missing its stable version")
    return tuple(map(int, match[1].split("."))) > tuple(map(int, tag[1:].split(".")))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag")
    parser.add_argument("archive", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument(
        "--source-url", help="Override the download URL for local testing"
    )
    parser.add_argument(
        "--only-newer", action="store_true", help="Keep newer tap releases"
    )
    args = parser.parse_args()
    template = Path(__file__).with_name("nvueschema.rb.in").read_text()
    try:
        formula = render(args.tag, args.archive, template, args.source_url)
        if args.only_newer and args.output.exists():
            if would_downgrade(args.output.read_text(), args.tag):
                print(f"Keeping the newer Homebrew formula instead of {args.tag}")
                return
    except ValueError as error:
        parser.error(str(error))
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(formula)


if __name__ == "__main__":
    main()
