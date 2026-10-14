# nvueschema

Generated Pydantic v2 models for Cumulus Linux NVUE, with an optional Go-backed
validator. Models cover the 5.x releases in the repository's version manifest.

```sh
uv add nvueschema          # Pydantic models
uv add 'nvueschema[cli]'  # Models and native CLI
uv add 'nvueschema[full]' # Models, native CLI, and Go-backed validation
```

The `cli` and `full` extras currently install the same binary package. Run
`uv run nvueschema --help` in your project, or install with
`pip install 'nvueschema[cli]'` in an activated virtual environment and run
`nvueschema --help`. The installed command is the native executable itself.
For the CLI alone, use `uv tool install nvueschema-binary` or
`pip install nvueschema-binary`.

```python
from nvueschema.models.v5_16 import NvueConfig

config = {"interface": {"swp1": {"link": {"mtu": 1500}}}}
model = NvueConfig.model_validate(config)
```

For the same schema checks as the Go CLI:

```python
from nvueschema import Validator, ValidationError

validator = Validator(version="5.16")  # downloads/caches schema on validation
# For offline use: Validator(spec="openapi-5.16.json")
try:
    validator.validate(config)  # None on success
except ValidationError as exc:
    print(exc.errors)  # path, schema_path (JSON Pointers), message

results = validator.validate_many(
    [config, {"interface": {"swp1": {"link": {"mtu": -1}}}}]
)
for result in results:
    print(result.valid, result.errors)
```

The Pydantic models are convenient typed representations, not an exact replacement
for JSON Schema validation. They may coerce values, ignore extra fields, and
simplify composite schemas. Python regex behavior also differs from ECMAScript.
Installing `full` does not alter model behavior. Pass original JSON-compatible
input to `Validator`, not a serialized model, so normalization cannot hide errors.
This checks the published schema, not device-side operational constraints.

Each validation call starts the bundled executable. `validate_many` amortizes
process startup and schema compilation across a batch, and returns a result for
every input. `BackendError` signals schema-loading, timeout, or executable failures;
`ValidationError` signals invalid configuration in `validate`. The timeout defaults
to 120 seconds and can be set with `Validator(..., timeout=30)`.

The default package needs only Python and Pydantic at runtime. The full extra adds
a platform-specific executable. Building the native package from source requires
Go; installing a supported native wheel does not.
