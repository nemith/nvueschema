"""Location of the bundled Go executable (private backend package)."""

import os
from pathlib import Path


def executable() -> Path:
    path = (
        Path(__file__).parent
        / "bin"
        / ("nvueschema.exe" if os.name == "nt" else "nvueschema")
    )
    if not path.is_file():
        raise FileNotFoundError(
            "Bundled nvueschema executable missing; reinstall nvueschema[full]"
        )
    return path
