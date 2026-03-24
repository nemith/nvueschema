package main

import (
	"strings"

	"github.com/nemith/nvueschema"
)

// subtreeMatches returns true if any descendant of n has a path or name
// matching query (already lowercased). Used by the filter to decide whether
// to keep ancestor nodes that don't themselves match.
func subtreeMatches(n *nvueschema.Node, pathPrefix, query string) bool {
	for _, child := range n.Children {
		collapsedName, eff := nvueschema.CollapseNode(child)
		path := collapsedName
		if pathPrefix != "" {
			path = pathPrefix + "." + collapsedName
		}
		if strings.Contains(strings.ToLower(path), query) ||
			strings.Contains(strings.ToLower(collapsedName), query) {
			return true
		}
		if subtreeMatches(eff, path, query) {
			return true
		}
	}
	return false
}

// nodeMatches reports whether this specific node's name or full path matches
// the filter query (already lowercased).
func nodeMatches(path, collapsedName, query string) bool {
	return strings.Contains(strings.ToLower(path), query) ||
		strings.Contains(strings.ToLower(collapsedName), query)
}
