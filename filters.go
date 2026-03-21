package main

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

// filterChanges keeps only changes whose paths match any of the given prefixes.
func filterChanges(changes []Change, filters []string) []Change {
	var out []Change
	for _, c := range changes {
		if pathMatches(c.Path, filters) {
			out = append(out, c)
		}
	}
	return out
}
