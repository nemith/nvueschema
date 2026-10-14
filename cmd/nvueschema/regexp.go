package main

import "nemith.io/nvueschema"

// JSON Schema uses ECMAScript patterns, including lookaround and
// backreferences, which Go's standard regexp engine cannot compile.
var compilePattern = nvueschema.CompilePattern
