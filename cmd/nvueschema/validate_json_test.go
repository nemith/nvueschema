package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateJSON(t *testing.T) {
	spec := filepath.Join(t.TempDir(), "spec.json")
	err := os.WriteFile(spec, []byte(`{"x-defs":{"cue-patch-schema-root-root":{"properties":{"profile":{"type":"string","pattern":"^(?!none$).*$"}}}}}`), 0600)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, input string
		batch       bool
		valid       []bool
	}{
		{"valid", `{"profile":"fast"}`, false, []bool{true}},
		{"invalid", `{"profile":"none"}`, false, []bool{false}},
		{"mixed", `[{"profile":"fast"},{"profile":"none"}]`, true, []bool{true, false}},
		{"empty", `[]`, true, []bool{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newValidateCmd()
			args := []string{"--json", "--config-format", "json", spec, "-"}
			if tc.batch {
				args = append(args, "--batch")
			}
			cmd.SetArgs(args)
			cmd.SetIn(strings.NewReader(tc.input))
			var out, stderr bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&stderr)
			err := cmd.Execute()
			allValid := true
			for _, valid := range tc.valid {
				allValid = allValid && valid
			}
			if (err == nil) != allValid {
				t.Fatalf("error %v, allValid %v", err, allValid)
			}
			var result validationResponse
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatalf("%v: %s", err, &out)
			}
			if result.Protocol != 1 || len(result.Results) != len(tc.valid) {
				t.Fatalf("%+v", result)
			}
			for i, valid := range tc.valid {
				if result.Results[i].Valid != valid || (len(result.Results[i].Issues) == 0) != valid {
					t.Fatalf("%+v", result.Results[i])
				}
			}
		})
	}
	for _, args := range [][]string{
		{"--batch", spec, "-"},
		{"--json", "--batch", "--config-format", "json", spec, "-"},
	} {
		cmd := newValidateCmd()
		cmd.SetArgs(args)
		cmd.SetIn(strings.NewReader(`{}`))
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err == nil {
			t.Fatal("accepted invalid batch invocation")
		}
		if output.Len() != 0 {
			t.Fatalf("unexpected success response: %s", &output)
		}
	}
}
