# nvueschema

Fetch the configuration schema from Cumulus Linux NVUE OpenAPI specs and view, diff, validate, or convert them to other formats.

![demo](demo.gif)

This tool exists because the only schema Nvidia provides is a full OpenAPI spec including all API endpoints, which makes it harder to reason about or validate just the configuration.

## Install

Choose Go, Python (pip or uv), Homebrew, or Nix below. All four can install the
CLI; the Python packages also provide Pydantic models and a validation API.

Standalone CLI archives are also available from [GitHub Releases](https://github.com/nemith/nvueschema/releases)
once the first release is published. Download the archive for your OS
(`linux`, `darwin` for macOS, or `windows`) and architecture (`amd64` or `arm64`),
extract it, and put `nvueschema` (`nvueschema.exe` on Windows) on your `PATH`.
These binaries require neither Go nor Python.

### Go

With Go installed:

```sh
go install nemith.io/nvueschema/cmd/nvueschema@latest
```

Ensure your Go binary installation directory (`GOBIN`, or `$(go env GOPATH)/bin`
by default) is on your `PATH`.

### Python (pip or uv)

Requires Python 3.10 or newer and the first PyPI release. Install Pydantic models
in your Python environment:

```sh
pip install nvueschema
# Or add to a uv project:
uv add nvueschema
```

To add the CLI to the models, use `pip install 'nvueschema[cli]'` or
`uv add 'nvueschema[cli]'`. For the full validation API and CLI:

```sh
pip install 'nvueschema[full]'
# Or add to a uv project:
uv add 'nvueschema[full]'
```

For just the CLI, without Pydantic models:

```sh
pip install nvueschema-binary
# Or install as a standalone uv tool:
uv tool install nvueschema-binary
nvueschema --help
```

The binary package installs the native executable directly into your Python
environment's `bin` directory (`Scripts` on Windows). Activate your virtual
environment or put that directory on `PATH`; in a uv project, use
`uv run nvueschema --help`. Supported wheels require no local Go installation.
The `cli` and `full` extras currently install the same backend, which serves
both the command and the Python API. See [Python usage and development](#python-packages-uv).

### Homebrew

After the packaging changes are merged and the first stable release is published,
install the CLI through Homebrew on macOS or Linux:

```sh
brew install nemith/tap/nvueschema
```

Homebrew builds the CLI from source and installs Go as a build dependency.

### Nix

With Nix flakes enabled, run or install a specific release directly from GitHub
on Linux or macOS (amd64 and arm64). Replace `v0.1.0` with a published release
containing the flake:

```sh
nix run github:nemith/nvueschema/v0.1.0 -- --help
nix profile add github:nemith/nvueschema/v0.1.0
```

For local development, `nix build path:.` and `nix run path:. -- --help` include
uncommitted files in the current checkout. Nix builds the CLI from source with
pinned nixpkgs and Go dependencies
on Linux and macOS (amd64 and arm64). Its version identifies the Git revision;
tagged flake URLs pin that revision without a separate version bump.

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

The [Tests workflow](.github/workflows/test.yml) runs on pull requests, pushes to
`main`, and manual dispatch. It installs the external validators for the unit
tests and runs a separate matrix job for every release in `testdata/schema-versions.json`.
Each release job downloads its schema with the NVUE fetcher, verifies its
checksum, checks every generator, and uploads its test log and coverage profile,
including when tests fail.
Matrix failures do not cancel the other releases.

CI sets `XDG_CACHE_HOME` to `$RUNNER_TEMP/cache` and uses GitHub Actions caching
for its `nvueschema` subdirectory, including `Last-Modified` metadata. Cache keys
include the release and manifest checksum. The fetcher still revalidates cached
schemas, and every run checks the downloaded or cached bytes against the manifest.

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

Failures in any generator, including YANG, are reported normally, not skipped
or marked as expected successes. Generator follow-ups are tracked in
[TODO.md](TODO.md).

## Python packages (uv)

The Python packages are developed in this repository with a shared uv workspace
and lockfile under `python/`. `nvueschema` contains generated Pydantic models;
the `cli` and `full` extras add `nvueschema-binary`, which installs the native
CLI and supplies the Go validator. The first PyPI release
must be published before the registry install commands below are available.

```sh
uv add nvueschema
# Or include the Go backend, without installing Go locally:
uv add 'nvueschema[full]'
```

```python
from nvueschema.models.v5_16 import NvueConfig
from nvueschema import Validator

config = {"interface": {"swp1": {"link": {"mtu": 1500}}}}
model = NvueConfig.model_validate(config)
Validator(version="5.16").validate(config)
# Offline: Validator(spec="openapi-5.16.json").validate(config)
```

Models ship for all releases in `testdata/schema-versions.json` and need no
runtime downloads. They provide Pydantic's convenient typed representation;
composite schemas, coercion, extra fields, and regex semantics can differ from
the Go validator. Installing `full` does not change model behavior. Validate
original input with the Go-backed API when exact CLI behavior is needed.
`validate_many(configs)` returns ordered results and compiles the schema once
per batch. See [the Python API documentation](python/nvueschema/README.md).

Development requires uv and Go. Generate the models, then install both workspace
packages and the test tools:

```sh
cd python
uv run --no-project python scripts/prepare_models.py
uv sync --locked --all-packages --all-extras
uv run --all-packages --all-extras pytest
uv run --all-packages --all-extras ruff check .
uv run --all-packages --all-extras ruff format --check .
uv build --all-packages
uv run --no-project python scripts/test_wheels.py
uv run --no-project python scripts/test_wheels.py --full
uv run --no-project python scripts/test_wheels.py --cli --installer pip
uv run --no-project python scripts/test_wheels.py --binary
```

Generated models and downloaded schemas are build products, not checked-in
source. Generation verifies the existing manifest's SHA256 checksums before
using either downloaded or cached schemas. Pass `--spec-dir PATH` to use an
existing schema directory without downloading. Standard package builds fail if
models have not been generated; editable installs allow generation afterward.
Hatchling supplies the wheel build hooks, while uv manages environments,
dependencies, builds, and publishing. Both source distributions are standalone:
the base includes generated models, and the native package includes Go sources.

The Python packages workflow tests default installs and native wheels on Linux
x86_64/aarch64, macOS x86_64/arm64, and Windows x86_64, using Python 3.10 and 3.14.
Native wheels have no CPython extension ABI dependency. Source builds of the
native package require Go. Linux wheels currently target glibc distributions.

Release versions come from GitHub release tags. Publishing a release tagged
`v0.1.0` builds and publishes both Python packages as `0.1.0`. CI sets both package
versions and the exact `cli`/`full` dependency pins in its build checkout; no version
edits or lockfile updates need to be committed for a release. The versions in the
checked-in package metadata are defaults for local development builds.

The workflow also runs a pinned `gorelease` check on PRs, pushes, and releases.
Both PyPI and TestPyPI uploads require it to pass. It compares the exported Go
API against the latest published stable Go module version below the proposed
release; the first release is checked without a baseline. Untagged CI runs
compare against the latest stable version and report a suggested next version.
The report appears in the workflow summary, including when the check fails.

Since the release tag already exists when CI runs, the check uses an unpublished
`-0.gorelease` prerelease with the same major/minor/patch numbers. This avoids
`gorelease` rejecting an already-published tag; it does not change the versions
published to PyPI. Under Go's pre-1.0 rules, incompatible API changes are reported
but do not themselves fail the check for `v0.x`. CLI flags, JSON output, and
validation behavior remain covered by tests rather than this API comparison.

Configure Trusted Publishing for
**both** `nvueschema` and `nvueschema-binary` on PyPI/TestPyPI using repository
`nemith/nvueschema` and workflow `python.yml`, with these GitHub environments:

| Project | PyPI environment | TestPyPI environment |
| --- | --- | --- |
| `nvueschema` | `pypi` | `testpypi` |
| `nvueschema-binary` | `pypi-binary` | `testpypi-binary` |

Each pending publisher needs a distinct configuration to create its project.
Create the matching GitHub environments with any desired release approval rules.
The workflow publishes each distribution in its own environment.

To release:

1. Merge the changes to release.
2. Open GitHub **Releases → Draft a new release** and choose a new tag such as
   `v0.1.0`, targeting the commit you want to ship. Add release notes.
3. Click **Publish release**. The **Python packages** workflow checks out the
   tagged commit, builds and tests the distributions, then publishes to PyPI
   and attaches CLI archives to the GitHub release.

GitHub releases include standalone `nvueschema` binaries for Linux, macOS
(`darwin`), and Windows, each on amd64 and arm64. Linux and macOS use `.tar.gz`;
Windows uses `.zip`. Each archive includes the license and README, and a SHA-256
checksums file covers all six archives. Run `nvueschema --version` to see the
embedded release version. These binaries do not require Python or Go installed.
GoReleaser builds them with cgo disabled. PRs and ordinary pushes build snapshot
archives as CI artifacts without uploading them to a GitHub release.

For a trial upload before merging, push a prerelease tag pointing at the PR
commit, such as `v0.1.0-alpha.1`. Alpha, beta, and RC tag pushes build, test, and
publish both packages to **TestPyPI**. They do not create a GitHub release or
update the Homebrew tap. Use a new prerelease number for each changed build;
TestPyPI does not allow replacing uploaded files.

After this workflow is on `main`, you can also manually run **Python packages**
with `publish=testpypi` and an existing `tag`, such as `v0.1.0`. Publishing a
GitHub release using that same tag publishes to production PyPI.

Prerelease tags use `v0.2.0-alpha.1`, `v0.2.0-beta.1`, or `v0.2.0-rc.1`; these
become Python versions `0.2.0a1`, `0.2.0b1`, and `0.2.0rc1`. Publishing a GitHub
prerelease also publishes to PyPI, so use a prerelease tag rather than relying
only on GitHub's prerelease checkbox. Draft releases, branch pushes, and PRs
do not publish. Package versions are independent of NVUE schema versions.

### Homebrew and Nix maintenance

The release workflow waits for Homebrew and Nix builds/tests on macOS and Linux
before publishing. After a stable release, it updates
[`nemith/homebrew-tap`](https://github.com/nemith/homebrew-tap) with a source
formula pinned to the release archive's SHA-256. Prereleases are skipped, and
releasing an older maintenance version will not downgrade the tap.

Tap updates use the `HOMEBREW_TAP_DEPLOY_KEY` Actions secret: an SSH private key
whose public key is registered as a write-enabled deploy key on
`nemith/homebrew-tap`. The key grants access only to that repository. To rotate
it, register a replacement deploy key, update the Actions secret, then remove
the old deploy key. No PyPI credentials are involved.

`packaging/homebrew/nvueschema.rb.in` is the formula template;
`packaging/homebrew/render.py` sets its version, source URL, and checksum.
Homebrew installs Go as a build dependency. CI installs the formula from a local
archive and tests both successful and unsuccessful configuration validation.

The Nix definition is in `nix/package.nix`, exposed through `flake.nix`.
The nixpkgs input uses the 26.05 stable branch to retain Intel macOS support.
After changing Go dependencies, build with `vendorHash = lib.fakeHash` once and
replace it with the actual hash reported by Nix, then run `nix flake check`.
Run `nix flake update nixpkgs` to refresh the pinned build tools in `flake.lock`.
`nix fmt flake.nix nix/package.nix` formats the definitions. Nix builds need no
publishing credentials; a shared binary cache is optional and is not configured.
