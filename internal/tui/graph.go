package tui

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
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
	all     bool // every branch, remote branch and tag (the default) instead of HEAD
	limit   int  // commits requested
	lines   []gitlog.GraphLine
	loaded  bool // lines hold the result for all and limit
	loading bool
	done    bool // the whole history is loaded
	err     error
	scroll  int
	cursor  int    // index of the selected commit line
	status  string // result of the last copy
}

// copyToClipboard writes to the system clipboard; tests replace it.
var copyToClipboard = clipboard.WriteAll

type copiedMsg struct {
	hash string
	err  error
}

func copyCommit(c *gitlog.GraphCommit) tea.Cmd {
	return func() tea.Msg {
		return copiedMsg{hash: c.FullHash, err: copyToClipboard(c.FullHash)}
	}
}

func (m *Model) receiveCopied(msg copiedMsg) {
	if msg.err != nil {
		m.graph.status = "Copy failed: " + msg.err.Error()
		return
	}
	m.graph.status = "Copied " + msg.hash
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
	if !g.isCommit(g.cursor) {
		g.cursor = g.nextCommit(g.cursor, 1)
	}
	m.clampGraph()
	return m.loadMoreGraph()
}

func (g *graphState) isCommit(i int) bool {
	return i >= 0 && i < len(g.lines) && g.lines[i].Commit != nil
}

// nextCommit returns the first commit line from i in direction dir (+1 or
// -1), falling back to the other direction, or 0 when there is none.
func (g *graphState) nextCommit(i, dir int) int {
	i = max(min(i, len(g.lines)-1), 0)
	for _, d := range []int{dir, -dir} {
		for j := i; j >= 0 && j < len(g.lines); j += d {
			if g.lines[j].Commit != nil {
				return j
			}
		}
	}
	return 0
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
	g.status = ""
	page := max(m.graphRows(), 1)
	switch key {
	case "up", "k":
		if j := g.nextCommit(g.cursor-1, -1); j < g.cursor {
			g.cursor = j
		}
	case "down", "j":
		if j := g.nextCommit(g.cursor+1, 1); j > g.cursor {
			g.cursor = j
		}
	case "pgup", "ctrl+u":
		g.cursor = g.nextCommit(g.cursor-page, -1)
	case "pgdown", "ctrl+d", " ":
		g.cursor = g.nextCommit(g.cursor+page, 1)
	case "home", "g":
		g.cursor = g.nextCommit(0, 1)
	case "end", "G":
		g.cursor = g.nextCommit(len(g.lines)-1, -1)
	case "enter":
		if g.isCommit(g.cursor) {
			return copyCommit(g.lines[g.cursor].Commit)
		}
		return nil
	case "a":
		*g = graphState{all: !g.all, limit: graphPage}
		return m.loadGraph()
	}
	m.clampGraph()
	return m.loadMoreGraph()
}

// clampGraph scrolls so the cursor stays visible.
func (m *Model) clampGraph() {
	g := &m.graph
	rows := max(m.graphRows(), 1)
	if g.cursor < g.scroll {
		g.scroll = g.cursor
	}
	if g.cursor >= g.scroll+rows {
		g.scroll = g.cursor - rows + 1
	}
	g.scroll = max(min(g.scroll, len(g.lines)-rows), 0)
}

func (m Model) graphRows() int {
	return m.height - graphChrome
}

func (m Model) graphScope() string {
	if m.graph.all {
		return "all branches"
	}
	if m.opts.Branch != "" {
		return "current branch " + m.opts.Branch
	}
	return "current branch"
}

func (m Model) graphView() string {
	t := m.theme
	g := m.graph
	if g.err != nil {
		return " " + t.dim.Render("Cannot read the graph: "+g.err.Error()) + "\n"
	}
	if !g.loaded && len(g.lines) == 0 {
		return " " + t.dim.Render("Loading graph of "+m.graphScope()+"…") + "\n"
	}
	var b strings.Builder
	end := min(g.scroll+m.graphRows(), len(g.lines))
	for i := g.scroll; i < end; i++ {
		b.WriteString(m.graphLine(g.lines[i], i == g.cursor) + "\n")
	}
	status := fmt.Sprintf("%d–%d of %d lines", g.scroll+1, end, len(g.lines))
	if !g.done {
		status += " · more loading as you scroll"
	}
	if g.status != "" {
		status = g.status
	}
	// The scope leads the status line so pressing a visibly changes it, even
	// when every branch is already merged into the current one.
	b.WriteString(" " + t.accent.Render(m.graphScope()) + t.dim.Render(" · "+status) + "\n")
	return b.String()
}

const (
	graphAuthorWidth = 16
	graphDateWidth   = 10
)

// graphLine renders one graph line; the selected commit is underlined from
// its hash to the end of the line.
func (m Model) graphLine(l gitlog.GraphLine, selected bool) string {
	t := m.theme
	c := l.Commit
	if c == nil {
		return " " + ansi.Truncate(l.Graph, max(m.width-2, 1), "")
	}
	u := func(s lipgloss.Style) lipgloss.Style { return s.Underline(selected) }
	space := func(n int) string { return u(t.text).Render(strings.Repeat(" ", max(n, 0))) }

	id := m.data.AuthorOf(c.Author, c.Email)
	hue := id
	if hue < 0 {
		hue = len(authorHues) // authors outside the counted history share grey
	}
	color := lipgloss.NewStyle().Foreground(t.authorColor(hue))
	faded := m.author != stats.AllAuthors && id != m.author

	// Lanes keep git's colors; the commit mark takes the author's.
	before, after, _ := strings.Cut(l.Graph, "*")
	graph := " " + before + color.Render("*") + after + " "

	subject := t.text
	if faded {
		subject = t.dim
	}
	text := u(t.dim).Render(c.Hash) + space(1)
	if len(c.Refs) > 0 {
		text += m.refs(c.Refs, selected) + space(1)
	}
	text += u(subject).Render(c.Subject)

	rightWidth := 2 + graphAuthorWidth + 2 + graphDateWidth + 1
	if m.width < 60+rightWidth {
		return graph + ansi.Truncate(text, max(m.width-1-lipgloss.Width(graph), 1), "…")
	}
	textWidth := m.width - rightWidth - lipgloss.Width(graph)
	text = ansi.Truncate(text, max(textWidth-1, 1), "…")
	name := ansi.Truncate(c.Author, graphAuthorWidth-2, "…")
	author := u(color).Render(cellGlyph) + space(1) + u(subject).Render(name)
	return graph + text + space(textWidth-lipgloss.Width(text)+2) +
		author + space(graphAuthorWidth-lipgloss.Width(author)+2) +
		u(t.dim).Render(c.Time.Local().Format("2006-01-02")) + " "
}

// refColors follow git's decoration colors.
var refColors = map[gitlog.RefKind]lipgloss.Color{
	gitlog.RefHead:   "#39c5cf",
	gitlog.RefBranch: "#3fb950",
	gitlog.RefRemote: "#f85149",
	gitlog.RefTag:    "#d29922",
	gitlog.RefOther:  "#d29922",
}

func (m Model) refs(refs []gitlog.Ref, underline bool) string {
	punct := m.theme.dim.Underline(underline)
	parts := make([]string, len(refs))
	for i, r := range refs {
		name := r.Name
		if r.Kind == gitlog.RefTag {
			name = "tag: " + name
		}
		parts[i] = lipgloss.NewStyle().Foreground(refColors[r.Kind]).Bold(true).Underline(underline).Render(name)
	}
	return punct.Render("(") + strings.Join(parts, punct.Render(", ")) + punct.Render(")")
}
