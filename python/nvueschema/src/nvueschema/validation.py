"""Explicit access to the Go validator; importing this module never starts it."""

from __future__ import annotations

import json
import os
import subprocess
from collections.abc import Iterable
from dataclasses import dataclass
from pathlib import Path
from typing import Any


class BackendError(RuntimeError):
    """The backend could not load a schema, execute, or complete its protocol."""


class BackendUnavailableError(BackendError):
    """Install the full extra to use the bundled backend."""


class ValidationError(ValueError):
    """Schema violations with JSON Pointer paths; no input coercion is performed."""

    def __init__(self, errors: list[dict[str, str]]):
        self.errors = errors
        super().__init__(
            "; ".join(f"{e['path'] or '/'}: {e['message']}" for e in errors)
        )


@dataclass(frozen=True)
class ValidationResult:
    valid: bool
    errors: list[dict[str, str]]


class Validator:
    """Validate original JSON-compatible input with the Go CLI's schema engine.

    Supply exactly one of version (download/cache an NVUE release) or spec
    (a local OpenAPI file). Each call starts a process; validate_many compiles
    the schema once for the whole batch. No process runs at construction time.
    """

    def __init__(
        self,
        *,
        version: str | None = None,
        spec: str | os.PathLike[str] | None = None,
        timeout: float = 120,
    ):
        if (version is None) == (spec is None):
            raise ValueError("supply exactly one of version or spec")
        if timeout <= 0:
            raise ValueError("timeout must be positive")
        if version is not None:
            import re

            if re.fullmatch(r"5\.\d+(?:\.\d+)?", version) is None:
                raise ValueError("version must be an NVUE 5.x or 5.x.y release")
            self._spec = version
        else:
            self._spec = str(Path(spec).resolve())
        self._timeout = timeout

    def validate(self, config: Any) -> None:
        """Raise ValidationError for invalid input, BackendError for tool failures."""
        result = self._run([config])[0]
        if not result.valid:
            raise ValidationError(result.errors)

    def validate_many(self, configs: Iterable[Any]) -> list[ValidationResult]:
        """Return ordered results for all inputs, including invalid configurations."""
        return self._run(list(configs))

    def _run(self, configs: list[Any]) -> list[ValidationResult]:
        # Reject Pydantic models and other non-JSON objects rather than silently
        # serializing/coercing them and changing what is being validated.
        payload = json.dumps(configs, allow_nan=False)
        try:
            from nvueschema_binary import executable
        except ImportError as exc:
            raise BackendUnavailableError(
                'Full validation requires the Go backend: uv add "nvueschema[full]" '
                '(or pip install "nvueschema[full]").'
            ) from exc
        try:
            process = subprocess.run(
                [
                    str(executable()),
                    "validate",
                    "--json",
                    "--batch",
                    "--config-format",
                    "json",
                    "--",
                    self._spec,
                    "-",
                ],
                input=payload,
                capture_output=True,
                text=True,
                encoding="utf-8",
                timeout=self._timeout,
                check=False,
            )
        except (OSError, subprocess.TimeoutExpired) as exc:
            raise BackendError(f"Unable to run NVUE validator: {exc}") from exc
        try:
            response = json.loads(process.stdout)
            if response["protocol"] != 1 or not isinstance(response["results"], list):
                raise ValueError("unsupported response protocol")
            rows = response["results"]
            if len(rows) != len(configs):
                raise ValueError("incorrect result count")
            results = []
            for row in rows:
                if type(row["valid"]) is not bool or not isinstance(
                    row["issues"], list
                ):
                    raise ValueError("invalid validation result")
                for issue in row["issues"]:
                    if not all(
                        isinstance(issue[k], str)
                        for k in ("path", "schema_path", "message")
                    ):
                        raise ValueError("invalid validation issue")
                if row["valid"] == bool(row["issues"]):
                    raise ValueError("inconsistent validation result")
                results.append(ValidationResult(row["valid"], row["issues"]))
            expected_status = 0 if all(r.valid for r in results) else 1
            if process.returncode != expected_status:
                raise ValueError(f"unexpected exit status {process.returncode}")
            return results
        except (ValueError, KeyError, TypeError) as exc:
            detail = process.stderr.strip() or str(exc)
            raise BackendError(f"NVUE backend failed: {detail}") from exc
