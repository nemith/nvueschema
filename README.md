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
