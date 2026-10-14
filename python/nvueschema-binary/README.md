# nvueschema-binary

Bundled Go executable for the `nvueschema` Python package. Install through
`uv add 'nvueschema[full]'`. This package contains no CPython extension and
requires no Go installation when installed from a wheel.

Source builds require Go and access to Go module dependencies. Wheels target
Linux x86_64/aarch64, macOS 12+ x86_64/arm64, and Windows x86_64.
