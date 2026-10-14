"""Location of the installed native CLI, also used by the Python validator."""

import os
from importlib.metadata import distribution
from pathlib import Path


def executable() -> Path:
    name = "nvueschema.exe" if os.name == "nt" else "nvueschema"
    package = distribution("nvueschema-binary")
    # RECORD tracks the actual install location, including user and editable
    # installs. Do not resolve from PATH, which may select a different version.
    for entry in package.files or ():
        if entry.name == name:
            path = Path(package.locate_file(entry))
            if path.is_file():
                return path
    raise FileNotFoundError(
        "Installed nvueschema executable missing; reinstall nvueschema-binary"
    )
