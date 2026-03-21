package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newValidateCmd() *cobra.Command {
	var (
		noCache   bool
		configFmt string
	)

	cmd := &cobra.Command{
		Use:   "validate <spec-or-version> <config-file>",
		Short: "Validate a config file against the NVUE schema",
		Long: strings.TrimSpace(`
Validate a YAML or JSON config file against the NVUE configuration schema.

The first argument is an OpenAPI spec file or a Cumulus Linux version
(e.g. "5.16"), which will be fetched/cached automatically.

The config file can be YAML or JSON (detected by extension, or
use --config-format to override).

Examples:
  nvueschema validate 5.16 config.yaml
  nvueschema validate 5.14 startup.json
  nvueschema validate 5.14 config.txt --config-format yaml
`),
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			ext, err := resolveSpec(args[0], noCache)
			if err != nil {
				return fmt.Errorf("loading spec: %w", err)
			}

			schema, err := ext.ConfigSchema()
			if err != nil {
				return fmt.Errorf("extracting config schema: %w", err)
			}

			jsDoc := schema.JSONSchemaDoc()

			jsBytes, err := json.Marshal(jsDoc)
			if err != nil {
				return fmt.Errorf("marshaling schema: %w", err)
			}

			var js jsonschema.Schema
			if err := json.Unmarshal(jsBytes, &js); err != nil {
				return fmt.Errorf("parsing schema: %w", err)
			}

			resolved, err := js.Resolve(nil)
			if err != nil {
				return fmt.Errorf("resolving schema: %w", err)
			}

			instance, err := loadConfig(args[1], configFmt)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			if err := resolved.Validate(instance); err != nil {
				fmt.Fprintf(os.Stderr, "Validation failed:\n%v\n", err)
				os.Exit(1)
			}

			fmt.Fprintln(os.Stderr, "Validation passed.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Skip cache entirely (don't read or write)")
	cmd.Flags().StringVar(&configFmt, "config-format", "", "Config file format: yaml or json (default: auto-detect from extension)")

	return cmd
}

func loadConfig(path, format string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(format)
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(path))
	}
	switch ext {
	case ".yaml", ".yml", "yaml", "yml":
		var v any
		if err := yaml.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("parsing YAML: %w", err)
		}
		return v, nil

	case ".json", "json":
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("parsing JSON: %w", err)
		}
		return v, nil

	default:
		var v any
		if err := yaml.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("could not parse %s as YAML or JSON: %w", path, err)
		}
		return v, nil
	}
}
