package main

import (
	"encoding/json"
	"fmt"
	"io"
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

	raw, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	var schemaMap map[string]any
	if err := json.Unmarshal(raw, &schemaMap); err != nil {
		return err
	}

	doc := map[string]any{
		"openapi": "3.0.3",
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

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
