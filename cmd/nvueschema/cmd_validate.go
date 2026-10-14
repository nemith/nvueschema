package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"nemith.io/nvueschema"
)

// Protocol 1 always returns one result per input configuration, in input order.
type validationResult struct {
	Valid  bool                         `json:"valid"`
	Issues []nvueschema.ValidationIssue `json:"issues"`
}
type validationResponse struct {
	Protocol int                `json:"protocol"`
	Results  []validationResult `json:"results"`
}

func newValidateCmd() *cobra.Command {
	var noCache, jsonOutput, batch bool
	var configFmt string
	cmd := &cobra.Command{
		SilenceUsage: true,
		Use:          "validate <spec-or-version> <config-file>",
		Short:        "Validate a config file against the NVUE schema",
		Long:         "Validate a YAML or JSON configuration against an NVUE spec file or version.\nUse - for stdin. With --batch, input is an array of configurations.\nUse --json for structured validation results (protocol version 1).",
		Args:         cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if batch && !jsonOutput {
				return errors.New("--batch requires --json")
			}
			parser, err := resolveSpec(args[0], noCache)
			if err != nil {
				return fmt.Errorf("loading spec: %w", err)
			}
			config, err := parser.ConfigSchema()
			if err != nil {
				return fmt.Errorf("extracting config schema: %w", err)
			}
			validator, err := nvueschema.NewValidator(config)
			if err != nil {
				return err
			}
			var instance any
			if args[1] == "-" {
				data, readErr := io.ReadAll(cmd.InOrStdin())
				if readErr != nil {
					return fmt.Errorf("reading stdin: %w", readErr)
				}
				instance, err = parseConfig(data, "stdin", configFmt)
			} else {
				instance, err = loadConfig(args[1], configFmt)
			}
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			instances := []any{instance}
			if batch {
				var ok bool
				instances, ok = instance.([]any)
				if !ok {
					return errors.New("--batch input must be an array of configurations")
				}
			}
			response := validationResponse{Protocol: 1, Results: make([]validationResult, 0, len(instances))}
			valid := true
			for _, value := range instances {
				result := validationResult{Valid: true, Issues: []nvueschema.ValidationIssue{}}
				if err := validator.Validate(value); err != nil {
					var invalid *nvueschema.ValidationError
					if !errors.As(err, &invalid) {
						return err
					}
					result.Valid, result.Issues, valid = false, invalid.Issues, false
				}
				response.Results = append(response.Results, result)
			}
			if jsonOutput {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(response); err != nil {
					return err
				}
			} else if valid {
				fmt.Fprintln(cmd.ErrOrStderr(), "Validation passed.")
			} else {
				fmt.Fprintln(cmd.ErrOrStderr(), "Validation failed:")
				for _, issue := range response.Results[0].Issues {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s (%s)\n", issue.Path, issue.Message, issue.SchemaPath)
				}
			}
			if !valid {
				cmd.SilenceUsage, cmd.SilenceErrors = true, true
				return errors.New("configuration validation failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Skip schema cache")
	cmd.Flags().StringVar(&configFmt, "config-format", "", "Config file format: yaml or json (default: auto-detect)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Return machine-readable validation results")
	cmd.Flags().BoolVar(&batch, "batch", false, "Validate an array of configurations using one compiled schema (requires --json)")
	return cmd
}
