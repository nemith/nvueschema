package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderDetail builds the detail pane content for the selected row.
func renderDetail(row flatRow, width int) string {
	if width < 2 {
		return ""
	}
	contentWidth := width - 2 // account for padding

	var sections []string

	// Name / Path.
	sections = append(sections, detailField("Name", row.collapsedName, contentWidth))
	sections = append(sections, detailField("Path", row.path, contentWidth))

	eff := row.effective

	// Type.
	if len(eff.TypeSegs) > 0 {
		var typeParts []string
		for _, seg := range eff.TypeSegs {
			if seg.Literal {
				typeParts = append(typeParts, styleLiteral.Render(seg.Text))
			} else {
				typeParts = append(typeParts, styleType.Render(seg.Text))
			}
		}
		sections = append(sections, detailField("Type", strings.Join(typeParts, ""), contentWidth))
	}

	// Default.
	if eff.DefaultVal != "" {
		sections = append(sections, detailField("Default", styleDefault.Render(eff.DefaultVal), contentWidth))
	}

	// Description.
	if eff.Desc != "" {
		desc := wordWrap(eff.Desc, contentWidth-14)
		sections = append(sections, detailField("Description", desc, contentWidth))
	}

	// Children summary.
	if len(eff.Children) > 0 {
		sections = append(sections, detailField("Children", fmt.Sprintf("%d", len(eff.Children)), contentWidth))
	}

	// Expand state.
	hasChildren := len(eff.Children) > 0
	if hasChildren {
		state := "collapsed"
		if row.expanded {
			state = "expanded"
		}
		sections = append(sections, detailField("State", state, contentWidth))
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func detailField(label, value string, _ int) string {
	return styleDetailLabel.Render(label+":") + " " + value
}

// wordWrap wraps text at word boundaries to fit within maxWidth.
func wordWrap(s string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 40
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}

	var lines []string
	line := words[0]
	for _, w := range words[1:] {
		if len(line)+1+len(w) > maxWidth {
			lines = append(lines, line)
			line = strings.Repeat(" ", 13) + w // indent continuation
		} else {
			line += " " + w
		}
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n")
}
