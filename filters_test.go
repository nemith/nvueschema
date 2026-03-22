package nvueschema

import (
	"os"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

func loadTestConfig(t *testing.T) any {
	t.Helper()
	data, err := os.ReadFile("testdata/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestConfigLeafPaths_NoSchema(t *testing.T) {
	config := loadTestConfig(t)

	// Without a schema, concrete keys are used as-is (no [*] replacement).
	paths := ConfigLeafPaths(config)
	sort.Strings(paths)

	// Should contain concrete paths like "interface.swp1.ipv4.vrrp..."
	found := false
	for _, p := range paths {
		if p == "router.bgp.asn" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected router.bgp.asn in paths, got %v", paths)
	}
}

func TestAffectMatches(t *testing.T) {
	configPaths := []string{
		"interface.[*].ipv4.vrrp.virtual-router.[*].version",
		"router.bgp.asn",
	}

	tests := []struct {
		changePath string
		want       bool
	}{
		// Exact match
		{"interface.[*].ipv4.vrrp.virtual-router.[*].version", true},
		{"router.bgp.asn", true},
		// Change is parent of config path
		{"interface.[*].ipv4.vrrp.virtual-router.[*]", true},
		{"interface.[*].ipv4", true},
		{"router.bgp", true},
		// Change is child of config path
		{"router.bgp.asn.something", true},
		// No overlap
		{"interface.[*].ipv6", false},
		{"router.ospf", false},
		{"vrf", false},
	}

	for _, tt := range tests {
		got := affectMatches(tt.changePath, configPaths)
		if got != tt.want {
			t.Errorf("affectMatches(%q) = %v, want %v", tt.changePath, got, tt.want)
		}
	}
}
