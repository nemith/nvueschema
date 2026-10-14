"""Build Go without cgo and package it independently of Python's extension ABI."""

import os
import platform
import subprocess
from pathlib import Path

from hatchling.builders.hooks.plugin.interface import BuildHookInterface


class CustomBuildHook(BuildHookInterface):
    def initialize(self, version, build_data):
        root = Path(self.root)
        source = root / "_go"
        if not source.is_dir():
            source = root.parents[1]
        if not (source / "go.mod").is_file():
            raise RuntimeError("Go sources missing from native package")
        if self.target_name == "sdist":
            # Make source distributions independently buildable outside the repo.
            for name in ("go.mod", "go.sum", "LICENSE"):
                build_data["force_include"][str(source / name)] = f"_go/{name}"
            for path in source.glob("*.go"):
                build_data["force_include"][str(path)] = f"_go/{path.name}"
            for directory in ("cmd/nvueschema", "yangregexp"):
                for path in (source / directory).glob("*.go"):
                    build_data["force_include"][str(path)] = (
                        f"_go/{directory}/{path.name}"
                    )
            return
        machine = platform.machine().lower()
        arch = {
            "arm64": "arm64",
            "aarch64": "arm64",
            "x86_64": "amd64",
            "amd64": "amd64",
        }.get(machine)
        system = platform.system()
        tags = {
            ("Linux", "amd64"): "manylinux_2_17_x86_64",
            ("Linux", "arm64"): "manylinux_2_17_aarch64",
            ("Darwin", "amd64"): "macosx_12_0_x86_64",
            ("Darwin", "arm64"): "macosx_12_0_arm64",
            ("Windows", "amd64"): "win_amd64",
        }
        if (system, arch) not in tags:
            raise RuntimeError(f"Unsupported native wheel platform: {system}/{machine}")
        target = (
            root
            / "src/nvueschema_native/bin"
            / ("nvueschema.exe" if system == "Windows" else "nvueschema")
        )
        target.parent.mkdir(parents=True, exist_ok=True)
        env = dict(
            os.environ,
            CGO_ENABLED="0",
            GOOS={"Linux": "linux", "Darwin": "darwin", "Windows": "windows"}[system],
            GOARCH=arch,
        )
        subprocess.run(
            [
                "go",
                "build",
                "-trimpath",
                "-buildvcs=false",
                "-ldflags=-s -w",
                "-o",
                str(target),
                "./cmd/nvueschema",
            ],
            cwd=source,
            env=env,
            check=True,
        )
        build_data["pure_python"] = False
        build_data["tag"] = "py3-none-" + tags[system, arch]
        build_data["artifacts"].append("src/nvueschema_native/bin/*")
