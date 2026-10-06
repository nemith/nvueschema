# nvueschema

Fetch the configuration schema from Cumulus Linux NVUE OpenAPI specs and view, diff, validate, or convert them to other formats.

![demo](demo.gif)

This tool exists because the only schema Nvidia provides is a full OpenAPI spec including all API endpoints, which makes it harder to reason about or validate just the configuration.

## Install

```
go install github.com/nemith/nvueschema/cmd/nvueschema@latest
```

## Usage

Anywhere a spec is expected, you can pass a version number instead of a file path. Specs are cached locally in `~/.cache/nvueschema` and validated with `If-Modified-Since`. Use `--no-cache` to skip the cache.

```
# Download the 5.16 spec from Nvidia and save it as spec.json
nvueschema fetch 5.16 -o spec.json

# Show a tree of all the `bridge` options in the 5.16 spec
nvueschema show 5.16 --path bridge

# Show a tree of differences between version 5.14 to 5.16
nvueschema diff 5.14 5.16

# Show a flat (one change per line) differences between 5.15 and 5.16 only for the interface top-level
nvueschema diff 5.15 5.16 --path interface -O flat

# Validate that the config.yaml file is valid for version 5.16
nvueschema validate 5.16 config.yaml

# Generate different config schemas
nvueschema gen -f pydantic 5.16 -o nvue.py
nvueschema gen -f yang 5.14
nvueschema gen -f proto --validate 5.16
nvueschema gen -f go 5.16 -o nvue.go
```

## Output formats
The supported output schemas for the `gen` command.

| Format | Flag | Notes |
|---|---|---|
| JSON Schema | `jsonschema`, `js` | Draft 2020-12 with `$defs` for format types |
| Pydantic | `pydantic`, `py` | v2 models with `Field(pattern=...)` validation |
| YANG | `yang` | Module with typedefs, `inet:` types, `leaf-list` |
| OpenAPI | `openapi`, `oas` | Minimal 3.1 spec, config schema only |
| Go | `go`, `golang` | Structs with `json`/`yaml` tags, `net/netip` types |
| Protobuf | `protobuf`, `proto` | Proto3 messages, optional `--validate` for buf protovalidate |

All formats include pattern-validated types for MAC addresses, interface names, route distinguishers, BGP communities, etc.

## Library
You can also use the Go package directly.

```go
import nvue "github.com/nemith/nvueschema"

p, _ := nvue.NewParser(reader)
cfg, _ := p.ConfigSchema()

// Generate
nvue.WriteYANG(os.Stdout, cfg, p.Info())

// Diff
diff := nvue.DiffSchemas(oldCfg, newCfg, "")
for _, c := range diff.Changes {
    fmt.Println(c.Kind, c.Path)
}

// Validate
doc := cfg.JSONSchemaDoc()
```

`JSONSchemaDoc` inherits enum constraints from `allOf`. For `anyOf` and
`oneOf`, it combines enum values only when every alternative is enum-constrained.
Unrestricted alternatives retain their value range and format types.

For example, the 5.18 schema accepts `router.bgp.state: enabled` or `disabled`,
but rejects legacy `on` and `off` values. Library consumers must use values
allowed by the generated schema.

## Tests

Run `go test ./...` for the Go regression tests. Generator integration tests
also compile Protobuf when `protoc` is on `PATH`, and validate YANG/Pydantic
when their Python packages are available. Missing external tools are reported
as skipped tests.

To run the Python integration tests in an isolated environment:

```sh
python3 -m venv /tmp/nvueschema-tests
/tmp/nvueschema-tests/bin/pip install pyang pydantic
NVUESCHEMA_PYTHON=/tmp/nvueschema-tests/bin/python go test ./...
```

An explicitly configured `NVUESCHEMA_PYTHON` must contain the required packages;
missing packages then fail the tests instead of skipping them.

### NVUE version matrix

`TestSchemaVersions` covers every 5.x release series from 5.0 through 5.18,
listed with source URLs and SHA-256 checksums in
[`testdata/schema-versions.json`](testdata/schema-versions.json).
It uses local downloads so ordinary unit tests do not require network access.
To prepare those downloads and the Protobuf validation definitions:

```sh
mkdir -p /tmp/nvueschema-specs
go build -o /tmp/nvueschema-tests-cli ./cmd/nvueschema
for minor in $(seq 0 18); do
  /tmp/nvueschema-tests-cli fetch "5.$minor" --no-cache \
    -o "/tmp/nvueschema-specs/openapi-5.$minor.json"
done
buf export buf.build/bufbuild/protovalidate --output /tmp/nvueschema-proto
```

With `go` and `protoc` on PATH and the Python environment above installed:

```sh
NVUESCHEMA_SPEC_DIR=/tmp/nvueschema-specs \
NVUESCHEMA_PYTHON=/tmp/nvueschema-tests/bin/python \
NVUESCHEMA_PROTO_INCLUDE=/tmp/nvueschema-proto \
  go test -count=1 -coverpkg=./... -coverprofile=/tmp/nvueschema-coverage.out ./...
go tool cover -html=/tmp/nvueschema-coverage.out
```

Enabling the matrix makes missing schemas, changed checksums, and missing
validator dependencies fail the tests. If NVIDIA updates a published schema,
verify the release before updating its manifest checksum.

Each release exercises the parser and CLI generation, compiles generated Go
and Protobuf (with and without validation annotations), imports Pydantic models,
and runs pyang on YANG. JSON Schema, the OpenAPI configuration component, and
Pydantic also check valid and invalid MTUs. The OpenAPI check validates the
configuration schema and its references, not the entire OpenAPI document.

The full matrix currently fails on YANG for all 19 releases; the remaining
generator defects are tracked in [TODO.md](TODO.md). These failures are reported
normally, not skipped or marked as expected successes.
