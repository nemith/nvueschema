package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"nemith.io/nvueschema"
)

// flatRow is a single visible row in the flattened tree.
type flatRow struct {
	node     *nvueschema.Node
	depth    int
	path     string // full dotted path
	expanded bool
	isLast   []bool // per-depth, for drawing tree connectors
	// collapsedName is the display name after CollapseNode.
	collapsedName string
	// effective is the node after collapsing (may differ from node).
	effective *nvueschema.Node
	// match is true when this node directly matches the active filter.
	match bool
}

// treeState holds the tree and its expand/collapse state.
type treeState struct {
	root        *nvueschema.Node
	expanded    map[*nvueschema.Node]bool
	rows        []flatRow
	filterQuery string // when non-empty, only matching subtrees are shown
}

func newTreeState(root *nvueschema.Node) treeState {
	ts := treeState{
		root:     root,
		expanded: make(map[*nvueschema.Node]bool),
	}
	// Expand root by default.
	ts.expanded[root] = true
	ts.rebuild()
	return ts
}

// rebuild recomputes the flat row list from the tree state.
func (ts *treeState) rebuild() {
	ts.rows = ts.rows[:0]
	ts.flatten(ts.root, 0, "", nil)
}

func (ts *treeState) flatten(n *nvueschema.Node, depth int, pathPrefix string, isLast []bool) {
	// When filtering, collect only the children that match or have matching descendants.
	children := n.Children
	if ts.filterQuery != "" {
		children = ts.filterChildren(n, pathPrefix)
	}

	for i, child := range children {
		collapsedName, effective := nvueschema.CollapseNode(child)

		path := collapsedName
		if pathPrefix != "" {
			path = pathPrefix + "." + collapsedName
		}

		last := i == len(children)-1
		childIsLast := append(append([]bool{}, isLast...), last)

		expanded := ts.expanded[child]
		hasChildren := len(effective.Children) > 0
		isMatch := false

		if ts.filterQuery != "" {
			isMatch = nodeMatches(path, collapsedName, ts.filterQuery)
			// Auto-expand nodes that are ancestors of matches (they don't
			// directly match but have matching descendants). Nodes that
			// directly match keep their current expand state so the user
			// can drill in manually.
			if !isMatch && hasChildren {
				expanded = true
				ts.expanded[child] = true
			}
		}

		ts.rows = append(ts.rows, flatRow{
			node:          child,
			depth:         depth,
			path:          path,
			expanded:      expanded && hasChildren,
			isLast:        childIsLast,
			collapsedName: collapsedName,
			effective:     effective,
			match:         isMatch,
		})

		if expanded && hasChildren {
			ts.flatten(effective, depth+1, path, childIsLast)
		}
	}
}

// filterChildren returns only the children of n whose subtree contains a match.
func (ts *treeState) filterChildren(n *nvueschema.Node, pathPrefix string) []*nvueschema.Node {
	q := strings.ToLower(ts.filterQuery)
	var kept []*nvueschema.Node
	for _, child := range n.Children {
		collapsedName, eff := nvueschema.CollapseNode(child)
		path := collapsedName
		if pathPrefix != "" {
			path = pathPrefix + "." + collapsedName
		}
		// Keep if this node matches or any descendant matches.
		if strings.Contains(strings.ToLower(path), q) ||
			strings.Contains(strings.ToLower(collapsedName), q) ||
			subtreeMatches(eff, path, q) {
			kept = append(kept, child)
		}
	}
	return kept
}

// toggle flips expand state for the node at index i.
func (ts *treeState) toggle(i int) {
	if i < 0 || i >= len(ts.rows) {
		return
	}
	row := ts.rows[i]
	if len(row.effective.Children) == 0 {
		return
	}
	ts.expanded[row.node] = !ts.expanded[row.node]
	ts.rebuild()
}

// expand sets a node as expanded.
func (ts *treeState) expand(i int) {
	if i < 0 || i >= len(ts.rows) {
		return
	}
	row := ts.rows[i]
	if len(row.effective.Children) > 0 {
		ts.expanded[row.node] = true
		ts.rebuild()
	}
}

// collapse collapses the node at index i.
func (ts *treeState) collapse(i int) {
	if i < 0 || i >= len(ts.rows) {
		return
	}
	row := ts.rows[i]
	if ts.expanded[row.node] {
		ts.expanded[row.node] = false
		ts.rebuild()
	}
}

// parentIndex returns the index of the parent of row i, or -1.
func (ts *treeState) parentIndex(i int) int {
	if i < 0 || i >= len(ts.rows) {
		return -1
	}
	depth := ts.rows[i].depth
	for j := i - 1; j >= 0; j-- {
		if ts.rows[j].depth < depth {
			return j
		}
	}
	return -1
}

// expandChildren expands all immediate children of node at i.
func (ts *treeState) expandChildren(i int) {
	if i < 0 || i >= len(ts.rows) {
		return
	}
	row := ts.rows[i]
	ts.expanded[row.node] = true
	for _, child := range row.effective.Children {
		cName, eff := nvueschema.CollapseNode(child)
		_ = cName
		if len(eff.Children) > 0 {
			ts.expanded[child] = true
		}
	}
	ts.rebuild()
}

// expandAll sets every node in the tree as expanded.
func (ts *treeState) expandAll() {
	ts.walkSetExpand(ts.root, true)
	ts.rebuild()
}

// collapseAll collapses every node except root.
func (ts *treeState) collapseAll() {
	for k := range ts.expanded {
		if k != ts.root {
			delete(ts.expanded, k)
		}
	}
	ts.rebuild()
}

func (ts *treeState) walkSetExpand(n *nvueschema.Node, expand bool) {
	if expand {
		ts.expanded[n] = true
	} else {
		delete(ts.expanded, n)
	}
	for _, child := range n.Children {
		_, eff := nvueschema.CollapseNode(child)
		if len(eff.Children) > 0 {
			ts.walkSetExpand(child, expand)
			ts.walkSetExpand(eff, expand)
		}
	}
}


// renderTreeLine renders a single tree row as a string.
func renderTreeLine(row flatRow, width int, isCursor bool) string {
	var b strings.Builder

	// Draw tree connectors.
	for d := 0; d < row.depth; d++ {
		if d < len(row.isLast) && row.isLast[d] {
			b.WriteString("    ")
		} else {
			b.WriteString("\u2502   ") // │
		}
	}

	// Connector for this node.
	lastIdx := len(row.isLast) - 1
	if lastIdx >= 0 && row.isLast[lastIdx] {
		b.WriteString("\u2514\u2500\u2500 ") // └──
	} else {
		b.WriteString("\u251c\u2500\u2500 ") // ├──
	}

	// Expand indicator for nodes with children.
	hasChildren := len(row.effective.Children) > 0
	if hasChildren {
		if row.expanded {
			b.WriteString("\u25bc ") // ▼
		} else {
			b.WriteString("\u25b6 ") // ▶
		}
	}

	// Node name.
	name := row.collapsedName
	if row.match {
		b.WriteString(styleSearchHit.Render(name))
	} else {
		b.WriteString(styleNodeName.Render(name))
	}

	// Type info (compact for tree).
	if len(row.effective.TypeSegs) > 0 {
		b.WriteString(" ")
		for _, seg := range row.effective.TypeSegs {
			if seg.Literal {
				b.WriteString(styleLiteral.Render(seg.Text))
			} else {
				b.WriteString(styleType.Render(seg.Text))
			}
		}
	}

	line := b.String()

	if isCursor {
		// Pad to width and apply cursor highlight.
		visible := lipglossWidth(line)
		if visible < width {
			line += strings.Repeat(" ", width-visible)
		}
		line = styleCursorLine.Render(line)
	}

	return line
}

// lipglossWidth returns the printable width of a styled string.
func lipglossWidth(s string) int {
	return lipgloss.Width(s)
}
