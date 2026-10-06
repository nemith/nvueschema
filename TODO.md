# Code review follow-up

Each item gets a separate Jujutsu change, with regression tests committed alongside the fix.

- [x] 1. Make CLI validation handle NVUE regular expressions, including negative lookahead, without weakening validation.
- [x] 2. Fix Protobuf map-entry name collisions and message deduplication across nested scopes; compile generated fixtures.
- [x] 3. Emit valid YANG type statements; validate generated fixtures with a YANG parser.
- [x] 4. Preserve JSON Schema format constraints, local constraints, and nullability, including scalar unions.
- [x] 5. Preserve Pydantic field bounds, lengths, and patterns (including numeric bounds on mixed unions); exercise generated models with valid and invalid values.
- [x] 6. Detect changes to array items, map values, required properties, and nullability in schema diffs.
- [x] 7. Preserve cache validators on the first download and revalidate cached schemas on subsequent fetches.
- [x] 8. Visit each subtree once when expanding the browser tree; add deep-tree coverage and benchmarks.
- [x] 9. Emit object models referenced by array items in Go, Python, and Protobuf; compile/initialize generated fixtures.
- [x] 10. Propagate writer errors from Pydantic, YANG, and Protobuf generation, including short writes.

## Follow-up discovered during full-schema verification

- [x] Escape source patterns when emitting YANG string literals. Regression
  tests parse generated YANG with `pyang` and compare the recovered pattern
  values, covering both quote types, backslashes, newlines, tabs, and Unicode.
  The complete 5.16 schema now passes statement parsing, exposing the separate
  validation failures below.

Initial full-schema validation with pyang 2.7.1 reported 566 errors (many instances
of the same underlying defects) and one unused-import warning:

- [x] Fix YANG default serialization. Preserve string and numeric values without
  display quoting or named constants; validate parsed defaults with pyang.
- [x] Emit valid YANG numeric ranges with decimal endpoints and unsigned types
  for nonnegative integers. Validate boundary values through pyang, including
  the full signed and unsigned 64-bit ranges. The reported reversed ranges were
  caused by named constants being misread as numbers, not reversed source bounds.
- [x] Preserve numeric enum values as restrictions on numeric YANG types, including
  nullable enums and union members. Emit each union member's bounds instead of
  dropping its restrictions; validate allowed and rejected values with pyang.
  Null-only alternatives map to leaf absence instead of empty enumeration
  types; untyped numeric enum constraints retain their numeric values.
- [x] Supply decimal64 fraction-digits and compatible range restrictions, with
  parser-backed tests for fractional bounds, defaults, and union members.
  Use six fractional digits by default, adjusting for declared values, and
  return a field-specific error when precision and magnitude cannot both fit.
- [ ] Handle source regexes that are incompatible with YANG's XML Schema regex
  dialect. Correct quoting preserves the source pattern but does not translate
  lookahead or unsupported escapes. Define supported conversions and explicit
  errors for unsupported patterns, with semantic regression tests.
- [x] Add a repeatable 5.0–5.18 schema matrix covering parsing and every output
  format, including Protobuf validation annotations. Pin downloaded schemas by
  checksum and require all external validators when the matrix is enabled.
  All 19 versions now pass every output format, including YANG.
  OpenAPI coverage checks the configuration
  component and references, not the complete document.
- [x] Run the version matrix in CI. GitHub Actions reads the release manifest,
  installs all validators, and runs each release in a separate job. Failures
  remain visible without cancelling the other releases.
- [x] Preserve scalar unions inside single-reference `allOf` wrappers in YANG.
  Validate IPv4-or-`auto` values and defaults with pyang, including nested
  wrappers, and retain wrapper constraints and inherited metadata.
- [ ] Add focused source fixtures as the YANG defects above are fixed.
