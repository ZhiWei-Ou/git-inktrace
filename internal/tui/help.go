package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// binding is a key, or keys, and what it does.
type binding struct{ key, desc string }

// helpSection is a titled group of bindings on the help page, with the
// pages it applies to.
type helpSection struct {
	title, scope string
	keys         []binding
}

// helpSections lists every key, grouped by where it works.
var helpSections = []helpSection{
	{"General", "", []binding{
		{"tab ⇧tab", "next / previous page"},
		{"1 – 5", "jump to a page"},
		{"?", "this help"},
		{"q ctrl+c", "quit"},
	}},
	{"Time window", "all but graph", []binding{
		{"← → h l", "year back / forward"},
		{"t", "latest window"},
	}},
	{"Lists", "files graph owners", []binding{
		{"↑ ↓ k j", "select"},
		{"pgup pgdn", "page"},
		{"g G", "top / bottom"},
		{"enter", "inspect selection"},
	}},
	{"Graph", "", []binding{
		{"a", "current / all branches"},
		{"/", "search, tab completes @"},
		{"n N", "next / previous match"},
		{"esc", "clear search"},
		{"o", "copy full hash"},
		{"e", "reword message"},
	}},
	{"Owners", "", []binding{
		{"enter", "open directory or file"},
		{"esc ⌫", "up a directory"},
		{"w", "area by changes / lines"},
	}},
	{"Inspect", "", []binding{
		{"↑ ↓ pgup", "scroll, g G ends"},
		{"q esc", "back"},
	}},
}

// helpSections returns the help without the pages the author filter hides.
func (m Model) helpSections() []helpSection {
	if !m.filtered() {
		return helpSections
	}
	var out []helpSection
	for _, s := range helpSections {
		if s.title == "Owners" {
			continue
		}
		s.scope = strings.TrimSuffix(s.scope, " owners")
		s.keys = slices.Clone(s.keys)
		for i, k := range s.keys {
			if k.key == "1 – 5" {
				s.keys[i].key = fmt.Sprintf("1 – %d", len(m.views()))
			}
		}
		out = append(out, s)
	}
	return out
}

// helpKey handles a key while the help page is open: ? q and esc close it,
// ctrl+c still quits, and other keys do nothing.
func (m *Model) helpKey(key string) bool {
	switch key {
	case "?", "q", "esc":
		m.helpOpen = false
	case "ctrl+c":
		return true
	}
	return false
}

// helpPage lays the sections out in as many columns as fit, kept in order
// with the tallest column as short as possible, and centers them.
func (m Model) helpPage() string {
	t := m.theme
	const gap = 4
	keyWidth, colWidth := 0, 0
	sections := m.helpSections()
	for _, s := range sections {
		for _, k := range s.keys {
			keyWidth = max(keyWidth, lipgloss.Width(k.key))
		}
	}
	title := func(s helpSection) string {
		if s.scope == "" {
			return t.accent.Bold(true).Render(s.title)
		}
		return t.accent.Bold(true).Render(s.title) + t.dim.Render(" · "+s.scope)
	}
	for _, s := range sections {
		colWidth = max(colWidth, lipgloss.Width(title(s)))
		for _, k := range s.keys {
			colWidth = max(colWidth, keyWidth+2+lipgloss.Width(k.desc))
		}
	}

	var blocks [][]string
	for _, s := range sections {
		lines := []string{title(s)}
		for _, k := range s.keys {
			lines = append(lines, t.text.Bold(true).Render(padRight(k.key, keyWidth))+"  "+t.dim.Render(k.desc))
		}
		blocks = append(blocks, lines)
	}

	// Three columns at most keep the page from spanning wide terminals.
	cols := max(min((m.width-2+gap)/(colWidth+gap), len(blocks), 3), 1)
	var columns [][]string
	for _, group := range splitEvenly(blocks, cols) {
		var c []string
		for i, b := range group {
			if i > 0 {
				c = append(c, "")
			}
			c = append(c, b...)
		}
		columns = append(columns, c)
	}

	rows := 0
	for _, c := range columns {
		rows = max(rows, len(c))
	}
	width := len(columns)*colWidth + (len(columns)-1)*gap
	pad := strings.Repeat(" ", max((m.width-width)/2, 1))
	var b strings.Builder
	for r := 0; r < rows; r++ {
		line := pad
		for i, c := range columns {
			cell := ""
			if r < len(c) {
				cell = c[r]
			}
			if i < len(columns)-1 {
				cell = padRight(cell, colWidth+gap)
			}
			line += cell
		}
		b.WriteString(ansi.Truncate(strings.TrimRight(line, " "), max(m.width-1, 1), "…") + "\n")
	}
	return b.String()
}

// splitEvenly divides blocks, in order, into at most n runs so that the
// tallest run, counting a blank line between blocks, is as short as possible.
func splitEvenly(blocks [][]string, n int) [][][]string {
	height := func(run [][]string) int {
		h := len(run) - 1
		for _, b := range run {
			h += len(b)
		}
		return h
	}
	if n <= 1 || len(blocks) <= 1 {
		return [][][]string{blocks}
	}
	best, bestHeight := [][][]string{blocks}, height(blocks)
	for i := 1; i < len(blocks); i++ {
		rest := splitEvenly(blocks[i:], n-1)
		h := height(blocks[:i])
		for _, r := range rest {
			h = max(h, height(r))
		}
		if h < bestHeight {
			best, bestHeight = append([][][]string{blocks[:i]}, rest...), h
		}
	}
	return best
}

// footer shows the few keys that matter most on the current page, keys
// bright and their actions dim, and points to ? for the rest.
func (m Model) footer() string {
	var keys []binding
	switch {
	case m.helpOpen:
		keys = []binding{{"?", "close"}}
	case m.inspect.open:
		keys = []binding{{"↑↓", "scroll"}, {"q", "back"}, {"?", "help"}}
	case m.view == FilesView:
		keys = []binding{{"↑↓", "select"}, {"enter", "inspect"}, {"←→", "year"}, {"?", "help"}, {"q", "quit"}}
	case m.view == GraphView:
		keys = []binding{{"↑↓", "select"}, {"enter", "inspect"}, {"/", "search"}, {"e", "reword"}, {"?", "help"}, {"q", "quit"}}
	case m.view == OwnersView:
		keys = []binding{{"↑↓", "select"}, {"enter", "open"}, {"esc", "up"}, {"w", "area"}, {"?", "help"}, {"q", "quit"}}
	default: // heatmap and hours
		keys = []binding{{"←→", "year"}, {"tab", "next page"}, {"?", "help"}, {"q", "quit"}}
	}
	t := m.theme
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = t.text.Bold(true).Render(k.key) + " " + t.dim.Render(k.desc)
	}
	return " " + ansi.Truncate(strings.Join(parts, "   "), max(m.width-2, 0), "…")
}
