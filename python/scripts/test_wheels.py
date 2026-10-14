"""Install built wheels in isolation and run tests against the installed package."""

import argparse
import os
import subprocess
import tempfile
from pathlib import Path

WORKSPACE = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--full", action="store_true")
    mode.add_argument("--cli", action="store_true")
    mode.add_argument("--binary", action="store_true", help="Test the CLI-only package")
    parser.add_argument("--installer", choices=("uv", "pip"), default="uv")
    parser.add_argument("--python", default="3.10")
    args = parser.parse_args()
    pattern = "nvueschema_binary-*.whl" if args.binary else "nvueschema-*.whl"
    wheels = list((WORKSPACE / "dist").glob(pattern))
    if len(wheels) != 1:
        raise RuntimeError(
            f"Expected one {pattern} wheel in dist; clean old releases before testing"
        )
    with tempfile.TemporaryDirectory(prefix="nvueschema-wheel-") as directory:
        env = Path(directory) / "venv"
        subprocess.run(
            ["uv", "venv", str(env), "--python", args.python]
            + (["--seed"] if args.installer == "pip" else []),
            check=True,
        )
        python = env / ("Scripts/python.exe" if os.name == "nt" else "bin/python")
        extra = "[full]" if args.full else "[cli]" if args.cli else ""
        package = str(wheels[0]) + extra
        installer = (
            [str(python), "-m", "pip", "install"]
            if args.installer == "pip"
            else [
                "uv",
                "pip",
                "install",
                "--python",
                str(python),
                # Local rebuilds reuse a version; don't test a cached older wheel.
                "--refresh-package",
                "nvueschema-binary",
            ]
        )
        subprocess.run(
            installer
            + [
                "--find-links",
                str(WORKSPACE / "dist"),
                package,
                "pytest>=8,<10",
            ],
            check=True,
        )
        check = (
            "from nvueschema_binary import executable; assert executable().is_file()"
            if args.full or args.cli or args.binary
            else "import importlib.util; assert importlib.util.find_spec('nvueschema_binary') is None"
        )
        subprocess.run([str(python), "-c", check], cwd=directory, check=True)
        if args.binary:
            subprocess.run(
                [
                    str(python),
                    "-c",
                    "import importlib.util; assert importlib.util.find_spec('pydantic') is None",
                ],
                cwd=directory,
                check=True,
            )
        test_env = dict(
            os.environ,
            PATH=str(python.parent) + os.pathsep + os.environ.get("PATH", ""),
        )
        subprocess.run(
            [
                str(python),
                "-m",
                "pytest",
                "-q",
                "-c",
                str(WORKSPACE / "pyproject.toml"),
                str(
                    WORKSPACE / "tests" / "test_cli.py"
                    if args.binary
                    else WORKSPACE / "tests"
                ),
            ],
            cwd=directory,
            env=test_env,
            check=True,
        )
        if args.binary:
            tool_bin = Path(directory) / "tool-bin"
            tool_env = dict(
                os.environ,
                UV_TOOL_DIR=str(Path(directory) / "tools"),
                UV_TOOL_BIN_DIR=str(tool_bin),
                PATH=str(tool_bin) + os.pathsep + os.environ.get("PATH", ""),
            )
            subprocess.run(
                ["uv", "tool", "install", "--python", args.python, str(wheels[0])],
                env=tool_env,
                cwd=directory,
                check=True,
            )
            command = tool_bin / ("nvueschema.exe" if os.name == "nt" else "nvueschema")
            subprocess.run([str(command), "--version"], check=True, cwd=directory)
            subprocess.run(
                ["uv", "tool", "uninstall", "nvueschema-binary"],
                env=tool_env,
                cwd=directory,
                check=True,
            )
            assert not command.exists(), "uv tool uninstall left the CLI behind"


if __name__ == "__main__":
    main()
