# nvueschema-binary

Native `nvueschema` CLI, also used as the Go backend by the `nvueschema` Python
package. Install just the CLI:

```sh
uv tool install nvueschema-binary
# Or in an activated Python environment:
pip install nvueschema-binary
nvueschema --help
```

The wheel installs the Go executable directly into `bin` (`Scripts` on Windows),
with no Python launcher. This directory must be on your `PATH`. It contains one
copy of the executable, shared by the command and the Python validation API.
The CLI-only package has no Pydantic dependency.

For models plus the CLI and validation API, install `nvueschema[cli]` or
`nvueschema[full]` instead. Both extras currently select the same binary backend.
This package contains no CPython extension and requires no Go installation when
installed from a wheel.

Source builds require Go and access to Go module dependencies. Wheels target
Linux x86_64/aarch64, macOS 12+ x86_64/arm64, and Windows x86_64.
