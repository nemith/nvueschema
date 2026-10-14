"""Never publish a package missing its generated release models."""

import runpy
from pathlib import Path

from hatchling.builders.hooks.plugin.interface import BuildHookInterface


class CustomBuildHook(BuildHookInterface):
    def initialize(self, version, build_data):
        if version == "editable":
            return
        models = Path(self.root) / "src/nvueschema/models"
        manifest = models / "_versions.py"
        instruction = "From python/, run uv run --no-project python scripts/prepare_models.py first."
        if not manifest.is_file():
            raise RuntimeError(instruction)
        versions = runpy.run_path(str(manifest))["VERSIONS"]
        if not versions or any(
            not (models / f"v{v.replace('.', '_')}.py").is_file() for v in versions
        ):
            raise RuntimeError(instruction)
