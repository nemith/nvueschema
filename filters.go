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
