package main

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
)

// WriteOpenAPI outputs a minimal OpenAPI 3.0 spec containing only the config schema.
func WriteOpenAPI(w io.Writer, schema *Schema, info map[string]any) error {
	version := "1.0.0"
	if v, ok := info["version"].(string); ok {
		version = v
	}
	title := "Cumulus Linux NVUE Configuration"
	if v, ok := info["title"].(string); ok {
		title = v
	}

	// Reuse the same cleaned-up schema tree as JSON Schema output.
	schemaMap := schemaToJSONSchema(schema)

	doc := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       title + " (config schema only)",
			"version":     version,
			"description": fmt.Sprintf("Extracted configuration schema from NVUE OpenAPI spec version %s", version),
		},
		"paths": map[string]any{},
		"components": map[string]any{
			"schemas": map[string]any{
				"NvueConfig": schemaMap,
			},
		},
	}

	// OpenAPI 3.1 supports $defs inline in schemas, but for cleaner output
	// put format definitions under components/schemas.
	maps.Copy(doc["components"].(map[string]any)["schemas"].(map[string]any), formatDefs())

	// Rewrite $ref paths from #/$defs/ to #/components/schemas/ for OpenAPI.
	rewriteRefs(doc)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// rewriteRefs recursively rewrites $ref: "#/$defs/X" to "#/components/schemas/X".
func rewriteRefs(obj any) {
	switch v := obj.(type) {
	case map[string]any:
		if ref, ok := v["$ref"].(string); ok {
			if len(ref) > 7 && ref[:7] == "#/$defs" {
				v["$ref"] = "#/components/schemas" + ref[7:]
			}
		}
		for _, val := range v {
			rewriteRefs(val)
		}
	case []any:
		for _, val := range v {
			rewriteRefs(val)
		}
	}
}

func newOpenAPIFormat() *Format {
	return &Format{
		Name:        "openapi",
		Aliases:     []string{"oas"},
		Description: "Minimal OpenAPI 3.1 spec with config schema only",
		Write:       WriteOpenAPI,
	}
}
