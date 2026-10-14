"""Install built wheels in isolation and run tests against the installed package."""

import argparse
import os
import subprocess
import tempfile
from pathlib import Path

WORKSPACE = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--full", action="store_true")
    parser.add_argument("--python", default="3.10")
    args = parser.parse_args()
    wheels = list((WORKSPACE / "dist").glob("nvueschema-*.whl"))
    if len(wheels) != 1:
        raise RuntimeError(
            "Expected one nvueschema wheel in dist; clean old releases before testing"
        )
    with tempfile.TemporaryDirectory(prefix="nvueschema-wheel-") as directory:
        env = Path(directory) / "venv"
        subprocess.run(["uv", "venv", str(env), "--python", args.python], check=True)
        python = env / ("Scripts/python.exe" if os.name == "nt" else "bin/python")
        package = str(wheels[0]) + ("[full]" if args.full else "")
        subprocess.run(
            [
                "uv",
                "pip",
                "install",
                "--python",
                str(python),
                "--find-links",
                str(WORKSPACE / "dist"),
                package,
                "pytest>=8,<10",
            ],
            check=True,
        )
        check = (
            "from nvueschema_binary import executable; assert executable().is_file()"
            if args.full
            else "import importlib.util; assert importlib.util.find_spec('nvueschema_binary') is None"
        )
        subprocess.run([str(python), "-c", check], cwd=directory, check=True)
        subprocess.run(
            [
                str(python),
                "-m",
                "pytest",
                "-q",
                "-c",
                str(WORKSPACE / "pyproject.toml"),
                str(WORKSPACE / "tests"),
            ],
            cwd=directory,
            check=True,
        )


if __name__ == "__main__":
    main()
