package nvueschema

import "strings"

// pathMatches returns true if the change path is under any of the filter prefixes.
func pathMatches(changePath string, filters []string) bool {
	for _, f := range filters {
		if changePath == f || strings.HasPrefix(changePath, f+".") {
			return true
		}
	}
	return false
}

// affectMatches returns true if the change path overlaps with any config path.
// This is bidirectional: a change to a parent of a config path affects it,
// and a change to a child of a config path is also relevant.
func affectMatches(changePath string, configPaths []string) bool {
	for _, cp := range configPaths {
		if changePath == cp ||
			strings.HasPrefix(changePath, cp+".") ||
			strings.HasPrefix(cp, changePath+".") {
			return true
		}
	}
	return false
}

// ConfigLeafPaths walks a parsed config value (from YAML/JSON) alongside
// the schema tree and returns all paths in schema notation (with [*] for
// map keys). Both schemas should be provided to handle keys that may only
// exist in one version.
func ConfigLeafPaths(config any, schemas ...*Config) []string {
	seen := make(map[string]bool)
	if len(schemas) == 0 {
		configLeafPaths(config, nil, "", seen)
	} else {
		for _, schema := range schemas {
			configLeafPaths(config, schema, "", seen)
		}
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	return paths
}

func configLeafPaths(config any, schema *Config, prefix string, seen map[string]bool) {
	m, ok := config.(map[string]any)
	if !ok {
		// Leaf value — emit the current path.
		if prefix != "" {
			seen[prefix] = true
		}
		return
	}

	if len(m) == 0 {
		if prefix != "" {
			seen[prefix] = true
		}
		return
	}

	var flat *Config
	if schema != nil {
		flat = FlattenComposite(schema)
	}

	for key, val := range m {
		var childSchema *Config
		childKey := key

		if flat != nil {
			// Check fixed properties first.
			if flat.Properties != nil {
				if prop, ok := flat.Properties[key]; ok {
					childSchema = prop
				}
			}
			// If not a fixed property, treat as a map key -> [*].
			if childSchema == nil && flat.AdditionalProperties != nil {
				childKey = "[*]"
				childSchema = flat.AdditionalProperties
			}
		}

		childPath := childKey
		if prefix != "" {
			childPath = prefix + "." + childKey
		}

		configLeafPaths(val, childSchema, childPath, seen)
	}
}
