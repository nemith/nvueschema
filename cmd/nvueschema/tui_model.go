package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"nemith.io/nvueschema"
)

type model struct {
	tree      treeState
	cursor    int
	scrollOff int
	width     int
	height    int

	// Filter (prunes tree to matching subtrees, highlights direct matches).
	filtering   bool
	filterInput textinput.Model

	// Detail pane viewport.
	detailVP viewport.Model

	// Help overlay.
	showHelp bool

	// Version label for status bar.
	version string
}

func newModel(root *nvueschema.Node, version string) model {
	fi := textinput.New()
	fi.Prompt = "/"
	fi.CharLimit = 128

	m := model{
		tree:        newTreeState(root),
		filterInput: fi,
		version:     version,
	}
	return m
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.detailVP = viewport.New(m.detailWidth(), m.treeHeight())
		m.updateDetailContent()
		return m, nil

	case tea.KeyMsg:
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		if m.filtering {
			return m.updateFilter(msg)
		}

		return m.updateNormal(msg)
	}

	return m, nil
}

func (m model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)

	case "right", "enter", "l":
		m.tree.expand(m.cursor)
		m.clampCursor()
		m.updateDetailContent()
	case "left", "h":
		if len(m.tree.rows) == 0 {
			break
		}
		row := m.tree.rows[m.cursor]
		if m.tree.expanded[row.node] {
			m.tree.collapse(m.cursor)
			m.clampCursor()
		} else {
			// Jump to parent.
			if pi := m.tree.parentIndex(m.cursor); pi >= 0 {
				m.cursor = pi
				m.ensureVisible()
			}
		}
		m.updateDetailContent()

	case " ":
		m.tree.toggle(m.cursor)
		m.clampCursor()
		m.updateDetailContent()

	case "g":
		m.cursor = 0
		m.scrollOff = 0
		m.updateDetailContent()
	case "G":
		if len(m.tree.rows) > 0 {
			m.cursor = len(m.tree.rows) - 1
		}
		m.ensureVisible()
		m.updateDetailContent()

	case "ctrl+d":
		m.moveCursor(m.treeHeight() / 2)
	case "ctrl+u":
		m.moveCursor(-m.treeHeight() / 2)

	case "e":
		m.tree.expandChildren(m.cursor)
		m.clampCursor()
		m.updateDetailContent()
	case "E":
		m.tree.expandAll()
		m.clampCursor()
		m.updateDetailContent()
	case "c":
		m.tree.collapse(m.cursor)
		m.clampCursor()
		m.updateDetailContent()
	case "C":
		m.tree.collapseAll()
		m.cursor = 0
		m.scrollOff = 0
		m.updateDetailContent()

	case "/":
		m.filtering = true
		m.filterInput.Focus()
		m.filterInput.SetValue("")
		return m, textinput.Blink

	case "n":
		m.jumpToMatch(1)
	case "N":
		m.jumpToMatch(-1)

	case "escape":
		// Clear active filter.
		if m.tree.filterQuery != "" {
			m.tree.filterQuery = ""
			m.tree.rebuild()
			m.cursor = 0
			m.scrollOff = 0
			m.clampCursor()
			m.updateDetailContent()
		}

	case "?":
		m.showHelp = true
	}

	return m, nil
}

func (m model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		query := m.filterInput.Value()
		m.filtering = false
		m.filterInput.Blur()
		if query == "" {
			// Empty query clears the filter.
			m.tree.filterQuery = ""
			m.tree.rebuild()
		} else {
			m.tree.filterQuery = strings.ToLower(query)
			m.tree.rebuild()
		}
		m.cursor = 0
		m.scrollOff = 0
		m.clampCursor()
		// Jump to first match if any.
		m.jumpToMatch(1)
		m.updateDetailContent()
		return m, nil

	case "escape":
		m.filtering = false
		m.filterInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	return m, cmd
}

// jumpToMatch moves the cursor to the next (dir=1) or previous (dir=-1)
// row that directly matches the filter.
func (m *model) jumpToMatch(dir int) {
	if m.tree.filterQuery == "" || len(m.tree.rows) == 0 {
		return
	}
	n := len(m.tree.rows)
	for step := 1; step < n; step++ {
		idx := (m.cursor + step*dir + n) % n
		if m.tree.rows[idx].match {
			m.cursor = idx
			m.ensureVisible()
			m.updateDetailContent()
			return
		}
	}
}

func (m *model) moveCursor(delta int) {
	m.cursor += delta
	m.clampCursor()
	m.ensureVisible()
	m.updateDetailContent()
}

func (m *model) clampCursor() {
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.tree.rows) {
		m.cursor = len(m.tree.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *model) ensureVisible() {
	th := m.treeHeight()
	if th <= 0 {
		return
	}
	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+th {
		m.scrollOff = m.cursor - th + 1
	}
}

func (m *model) updateDetailContent() {
	if m.cursor >= 0 && m.cursor < len(m.tree.rows) {
		content := renderDetail(m.tree.rows[m.cursor], m.detailWidth())
		m.detailVP.SetContent(content)
	}
}

func (m model) treeHeight() int {
	h := m.height - 3 // status bar + border overhead
	if h < 1 {
		h = 1
	}
	return h
}

func (m model) treeWidth() int {
	if m.width < 60 {
		return m.width - 2 // no detail pane
	}
	return int(float64(m.width) * 0.55)
}

func (m model) detailWidth() int {
	if m.width < 60 {
		return 0
	}
	return m.width - m.treeWidth()
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	if m.showHelp {
		return m.renderHelp()
	}

	treePane := m.renderTreePane()
	statusBar := m.renderStatusBar()

	if m.detailWidth() > 0 {
		detailPane := m.renderDetailPane()
		main := lipgloss.JoinHorizontal(lipgloss.Top, treePane, detailPane)
		return lipgloss.JoinVertical(lipgloss.Left, main, statusBar)
	}

	return lipgloss.JoinVertical(lipgloss.Left, treePane, statusBar)
}

func (m model) renderTreePane() string {
	tw := m.treeWidth() - 2 // border
	th := m.treeHeight()

	var lines []string
	end := m.scrollOff + th
	if end > len(m.tree.rows) {
		end = len(m.tree.rows)
	}

	for i := m.scrollOff; i < end; i++ {
		row := m.tree.rows[i]
		isCursor := i == m.cursor
		line := renderTreeLine(row, tw, isCursor)
		lines = append(lines, line)
	}

	// Pad if fewer rows than height.
	for len(lines) < th {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	return stylePaneBorder.Width(tw).Height(th).Render(content)
}

func (m model) renderDetailPane() string {
	dw := m.detailWidth() - 2
	th := m.treeHeight()

	var content string
	if m.cursor >= 0 && m.cursor < len(m.tree.rows) {
		content = renderDetail(m.tree.rows[m.cursor], dw)
	}

	return stylePaneBorder.Width(dw).Height(th).Render(content)
}

func (m model) renderStatusBar() string {
	var parts []string

	parts = append(parts, styleStatusKey.Render(m.version))

	if m.cursor >= 0 && m.cursor < len(m.tree.rows) {
		parts = append(parts, m.tree.rows[m.cursor].path)
	}

	if m.filtering {
		parts = append(parts, m.filterInput.View())
	} else if m.tree.filterQuery != "" {
		// Count direct matches.
		matchCount := 0
		for _, row := range m.tree.rows {
			if row.match {
				matchCount++
			}
		}
		filterInfo := fmt.Sprintf("filter: %q (%d matches)", m.tree.filterQuery, matchCount)
		parts = append(parts, styleSearchHit.Render(filterInfo))
	}

	parts = append(parts, styleStatusKey.Render("? help"))

	bar := strings.Join(parts, "  \u2502  ")
	return styleStatusBar.Width(m.width).Render(bar)
}

func (m model) renderHelp() string {
	help := `
  Navigation
  ──────────
  ↑/k, ↓/j       Move cursor
  →/enter/l       Expand node
  ←/h             Collapse / go to parent
  space           Toggle expand/collapse
  g / G           Top / bottom
  ctrl+d/ctrl+u   Page down / up

  Tree
  ────
  e               Expand children
  E               Expand all
  c               Collapse node
  C               Collapse all

  Filter
  ──────
  /               Start filter (prunes tree)
  enter           Apply filter
  escape          Clear filter
  n / N           Next / prev match

  Other
  ─────
  ?               Toggle this help
  q / ctrl+c      Quit

  Press any key to close help.
`
	style := lipgloss.NewStyle().
		Padding(1, 3).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("4"))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		style.Render(help))
}
