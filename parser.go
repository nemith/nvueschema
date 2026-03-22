package nvueschema

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// rawSpec holds the top-level OpenAPI document with x-defs.
type rawSpec struct {
	OpenAPI string                    `json:"openapi"`
	Info    map[string]any            `json:"info"`
	XDefs   map[string]map[string]any `json:"x-defs"`
	Paths   map[string]map[string]any `json:"paths"`
}

// Config is a resolved JSON-Config-like node (no $ref remaining).
type Config struct {
	Description           string             `json:"description,omitempty"`
	Type                  string             `json:"type,omitempty"`
	Nullable              bool               `json:"nullable,omitempty"`
	Properties            map[string]*Config `json:"properties,omitempty"`
	AdditionalProperties  *Config            `json:"additionalProperties,omitempty"`
	Items                 *Config            `json:"items,omitempty"`
	AllOf                 []*Config          `json:"allOf,omitempty"`
	OneOf                 []*Config          `json:"oneOf,omitempty"`
	AnyOf                 []*Config          `json:"anyOf,omitempty"`
	Enum                  []any              `json:"enum,omitempty"`
	Default               any                `json:"default,omitempty"`
	Minimum               *float64           `json:"minimum,omitempty"`
	Maximum               *float64           `json:"maximum,omitempty"`
	MinLength             *int               `json:"minLength,omitempty"`
	MaxLength             *int               `json:"maxLength,omitempty"`
	Pattern               string             `json:"pattern,omitempty"`
	Format                string             `json:"format,omitempty"`
	Required              []string           `json:"required,omitempty"`
	XModelName            string             `json:"x-model-name,omitempty"`
	SourceRef string `json:"-"` // x-defs key or API path, not serialized
}

// Parser resolves $ref pointers within x-defs and extracts the config tree.
type Parser struct {
	raw       rawSpec
	resolved  map[string]*Config // cache of already-resolved x-defs
	stack     map[string]bool    // cycle detection
	defToPath map[string]string  // x-defs key -> API path (from PATCH requestBody)
}

// NewParser parses an OpenAPI JSON spec from an io.Reader.
func NewParser(r io.Reader) (*Parser, error) {
	var raw rawSpec
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("parsing spec: %w", err)
	}
	e := &Parser{
		raw:       raw,
		resolved:  make(map[string]*Config),
		stack:     make(map[string]bool),
		defToPath: make(map[string]string),
	}
	e.buildDefToPath()
	return e, nil
}

// buildDefToPath maps x-defs keys referenced by PATCH requestBody to their API path.
func (e *Parser) buildDefToPath() {
	for apiPath, methods := range e.raw.Paths {
		patch, ok := methods["patch"]
		if !ok {
			continue
		}
		patchMap, ok := patch.(map[string]any)
		if !ok {
			continue
		}
		body, ok := patchMap["requestBody"].(map[string]any)
		if !ok {
			continue
		}
		// Direct $ref on requestBody (e.g. "$ref": "#/x-defs/cue-patch-response-...")
		if ref, ok := body["$ref"].(string); ok {
			if name := refToDefName(ref); name != "" {
				e.defToPath[name] = apiPath
				// Also map the inner schema ref if we can resolve it
				if inner, ok := e.raw.XDefs[name]; ok {
					if content, ok := inner["content"].(map[string]any); ok {
						if appJSON, ok := content["application/json"].(map[string]any); ok {
							if schemaRef, ok := appJSON["schema"].(map[string]any); ok {
								if ref2, ok := schemaRef["$ref"].(string); ok {
									if name2 := refToDefName(ref2); name2 != "" {
										e.defToPath[name2] = apiPath
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

// ConfigSchema returns the fully-resolved config schema starting from
// the PATCH request body of the root path ("/").
func (e *Parser) ConfigSchema() (*Config, error) {
	return e.resolveRef("cue-patch-schema-root-root")
}

// Info returns the spec info block.
func (e *Parser) Info() map[string]any {
	return e.raw.Info
}

// ConfigPaths returns all paths that have a PATCH operation (i.e., configurable).
func (e *Parser) ConfigPaths() []string {
	var paths []string
	for p, methods := range e.raw.Paths {
		if _, ok := methods["patch"]; ok {
			paths = append(paths, p)
		}
	}
	return paths
}

func (e *Parser) resolveRef(defName string) (*Config, error) {
	if s, ok := e.resolved[defName]; ok {
		return s, nil
	}
	if e.stack[defName] {
		// Return a stub instead of recursing infinitely. The description
		// makes the cycle visible in show/diff output.
		return &Config{Description: fmt.Sprintf("WARNING: circular ref: %s", defName)}, nil
	}
	e.stack[defName] = true
	defer delete(e.stack, defName)

	raw, ok := e.raw.XDefs[defName]
	if !ok {
		return nil, fmt.Errorf("x-def %q not found", defName)
	}

	s, err := e.resolveNode(raw)
	if err != nil {
		return nil, fmt.Errorf("resolving %q: %w", defName, err)
	}
	// Tag with source: prefer API path, fall back to x-defs key.
	if apiPath, ok := e.defToPath[defName]; ok {
		s.SourceRef = apiPath
	} else {
		s.SourceRef = "x-defs/" + defName
	}
	e.resolved[defName] = s
	return s, nil
}

func (e *Parser) resolveNode(raw map[string]any) (*Config, error) {
	if ref, ok := raw["$ref"].(string); ok {
		name := refToDefName(ref)
		if name == "" {
			return nil, fmt.Errorf("unsupported $ref: %s", ref)
		}
		return e.resolveRef(name)
	}

	s := &Config{}

	if v, ok := raw["description"].(string); ok {
		s.Description = v
	}
	if v, ok := raw["type"].(string); ok {
		s.Type = v
	}
	if v, ok := raw["nullable"].(bool); ok {
		s.Nullable = v
	}
	if v, ok := raw["pattern"].(string); ok {
		s.Pattern = v
	}
	if v, ok := raw["format"].(string); ok {
		s.Format = v
	}
	if v, ok := raw["default"]; ok {
		s.Default = v
	}
	if v, ok := raw["x-model-name"].(string); ok {
		s.XModelName = v
	}
	if v, ok := raw["minimum"].(float64); ok {
		s.Minimum = &v
	}
	if v, ok := raw["maximum"].(float64); ok {
		s.Maximum = &v
	}
	if v, ok := raw["minLength"].(float64); ok {
		i := int(v)
		s.MinLength = &i
	}
	if v, ok := raw["maxLength"].(float64); ok {
		i := int(v)
		s.MaxLength = &i
	}

	if arr, ok := raw["enum"].([]any); ok {
		s.Enum = arr
	}
	if arr, ok := raw["required"].([]any); ok {
		for _, v := range arr {
			if str, ok := v.(string); ok {
				s.Required = append(s.Required, str)
			}
		}
	}

	if props, ok := raw["properties"].(map[string]any); ok {
		s.Properties = make(map[string]*Config, len(props))
		for k, v := range props {
			// Skip API action properties — not part of config state.
			if strings.HasPrefix(k, "@") {
				continue
			}
			node, ok := v.(map[string]any)
			if !ok {
				continue
			}
			resolved, err := e.resolveNode(node)
			if err != nil {
				return nil, fmt.Errorf("property %q: %w", k, err)
			}
			// Prune action-only containers: nodes with no type, no properties,
			// and no additionalProperties. These held only @-prefixed actions.
			if isEmptySchema(resolved) {
				continue
			}
			s.Properties[k] = resolved
		}
	}

	if ap, ok := raw["additionalProperties"].(map[string]any); ok {
		resolved, err := e.resolveNode(ap)
		if err != nil {
			return nil, fmt.Errorf("additionalProperties: %w", err)
		}
		s.AdditionalProperties = resolved
	}

	if items, ok := raw["items"].(map[string]any); ok {
		resolved, err := e.resolveNode(items)
		if err != nil {
			return nil, fmt.Errorf("items: %w", err)
		}
		s.Items = resolved
	}

	for _, key := range []string{"allOf", "oneOf", "anyOf"} {
		if arr, ok := raw[key].([]any); ok {
			var list []*Config
			for i, v := range arr {
				node, ok := v.(map[string]any)
				if !ok {
					continue
				}
				resolved, err := e.resolveNode(node)
				if err != nil {
					return nil, fmt.Errorf("%s[%d]: %w", key, i, err)
				}
				list = append(list, resolved)
			}
			switch key {
			case "allOf":
				s.AllOf = list
			case "oneOf":
				s.OneOf = list
			case "anyOf":
				s.AnyOf = list
			}
		}
	}

	return s, nil
}

// isEmptySchema returns true if a resolved schema has no type information,
// no properties, and no composition — i.e., it's an action-only container
// that contributes nothing to the config tree.
func isEmptySchema(s *Config) bool {
	flat := FlattenComposite(s)
	return flat.Type == "" &&
		len(flat.Properties) == 0 &&
		flat.AdditionalProperties == nil &&
		len(flat.Enum) == 0 &&
		!isScalarUnion(s)
}

func refToDefName(ref string) string {
	const prefix = "#/x-defs/"
	if strings.HasPrefix(ref, prefix) {
		return ref[len(prefix):]
	}
	return ""
}
