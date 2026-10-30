package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ZhiWei-Ou/git-inktrace/internal/gitlog"
	"github.com/ZhiWei-Ou/git-inktrace/internal/stats"
)

// graphPage is how many commits the graph loads at first and adds each time
// scrolling nears the end.
const graphPage = 500

// graphChrome is the lines around the graph: header, tabs, blank, status, blank, help.
const graphChrome = 6

type graphState struct {
	all     bool // every branch, remote branch and tag instead of HEAD
	limit   int  // commits requested
	lines   []gitlog.GraphLine
	loaded  bool // lines hold the result for all and limit
	loading bool
	done    bool // the whole history is loaded
	err     error
	scroll  int
}

type graphMsg struct {
	all   bool
	limit int
	lines []gitlog.GraphLine
	err   error
}

// loadGraph starts reading the graph unless it is loaded or loading.
func (m *Model) loadGraph() tea.Cmd {
	g := &m.graph
	if g.loaded || g.loading {
		return nil
	}
	g.loading = true
	dir, all, limit := m.opts.Dir, g.all, g.limit
	return func() tea.Msg {
		lines, err := gitlog.Graph(dir, all, limit)
		return graphMsg{all: all, limit: limit, lines: lines, err: err}
	}
}

func (m *Model) receiveGraph(msg graphMsg) tea.Cmd {
	g := &m.graph
	if msg.all != g.all || msg.limit != g.limit {
		return nil // a stale result after the scope changed
	}
	g.loading, g.loaded, g.err = false, true, msg.err
	g.lines = msg.lines
	commits := 0
	for _, l := range msg.lines {
		if l.Commit != nil {
			commits++
		}
	}
	g.done = msg.err != nil || commits < msg.limit
	m.clampGraph()
	return m.loadMoreGraph()
}

// loadMoreGraph extends the graph when the view nears the loaded end.
func (m *Model) loadMoreGraph() tea.Cmd {
	g := &m.graph
	if g.done || g.loading || g.scroll+2*m.graphRows() < len(g.lines) {
		return nil
	}
	g.limit += graphPage
	g.loaded = false
	return m.loadGraph()
}

func (m *Model) graphKey(key string) tea.Cmd {
	g := &m.graph
	page := max(m.graphRows(), 1)
	switch key {
	case "up", "k":
		g.scroll--
	case "down", "j":
		g.scroll++
	case "pgup", "ctrl+u":
		g.scroll -= page
	case "pgdown", "ctrl+d", " ":
		g.scroll += page
	case "home", "g":
		g.scroll = 0
	case "end", "G":
		g.scroll = len(g.lines)
	case "b":
		*g = graphState{all: !g.all, limit: graphPage}
		return m.loadGraph()
	}
	m.clampGraph()
	return m.loadMoreGraph()
}

func (m *Model) clampGraph() {
	g := &m.graph
	g.scroll = max(min(g.scroll, len(g.lines)-m.graphRows()), 0)
}

func (m Model) graphRows() int {
	return m.height - graphChrome
}

func (m Model) graphScope() string {
	if m.graph.all {
		return "all branches"
	}
	return "HEAD"
}

func (m Model) graphView() string {
	t := m.theme
	g := m.graph
	if g.err != nil {
		return " " + t.dim.Render("Cannot read the graph: "+g.err.Error()) + "\n"
	}
	if !g.loaded && len(g.lines) == 0 {
		return " " + t.dim.Render("Loading graph…") + "\n"
	}
	var b strings.Builder
	end := min(g.scroll+m.graphRows(), len(g.lines))
	for _, l := range g.lines[g.scroll:end] {
		b.WriteString(m.graphLine(l) + "\n")
	}
	status := fmt.Sprintf("%d–%d of %d lines", g.scroll+1, end, len(g.lines))
	if !g.done {
		status += " · more loading as you scroll"
	}
	b.WriteString(" " + t.dim.Render(status) + "\n")
	return b.String()
}

const (
	graphAuthorWidth = 16
	graphDateWidth   = 10
)

func (m Model) graphLine(l gitlog.GraphLine) string {
	t := m.theme
	c := l.Commit
	if c == nil {
		return " " + ansi.Truncate(l.Graph, max(m.width-2, 1), "")
	}
	id := m.data.AuthorOf(c.Author, c.Email)
	hue := id
	if hue < 0 {
		hue = len(authorHues) // authors outside the counted history share grey
	}
	color := lipgloss.NewStyle().Foreground(t.authorColor(hue))
	faded := m.author != stats.AllAuthors && id != m.author

	// Lanes keep git's colors; the commit mark takes the author's.
	before, after, _ := strings.Cut(l.Graph, "*")
	graph := before + color.Render("*") + after

	subject := t.text
	if faded {
		subject = t.dim
	}
	left := " " + graph + " " + t.dim.Render(c.Hash) + " "
	if len(c.Refs) > 0 {
		left += m.refs(c.Refs) + " "
	}
	left += subject.Render(c.Subject)

	rightWidth := 2 + graphAuthorWidth + 2 + graphDateWidth + 1
	if m.width < 60+rightWidth {
		return ansi.Truncate(left, max(m.width-1, 1), "…")
	}
	name := ansi.Truncate(c.Author, graphAuthorWidth-2, "…")
	author := color.Render(cellGlyph) + " " + subject.Render(name)
	right := padRight(author, graphAuthorWidth) + "  " + t.dim.Render(c.Time.Local().Format("2006-01-02")) + " "
	left = ansi.Truncate(left, m.width-rightWidth-1, "…")
	return padRight(left, m.width-rightWidth) + "  " + right
}

// refColors follow git's decoration colors.
var refColors = map[gitlog.RefKind]lipgloss.Color{
	gitlog.RefHead:   "#39c5cf",
	gitlog.RefBranch: "#3fb950",
	gitlog.RefRemote: "#f85149",
	gitlog.RefTag:    "#d29922",
	gitlog.RefOther:  "#d29922",
}

func (m Model) refs(refs []gitlog.Ref) string {
	t := m.theme
	parts := make([]string, len(refs))
	for i, r := range refs {
		name := r.Name
		if r.Kind == gitlog.RefTag {
			name = "tag: " + name
		}
		parts[i] = lipgloss.NewStyle().Foreground(refColors[r.Kind]).Bold(true).Render(name)
	}
	return t.dim.Render("(") + strings.Join(parts, t.dim.Render(", ")) + t.dim.Render(")")
}
