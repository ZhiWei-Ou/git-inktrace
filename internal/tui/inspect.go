package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/ZhiWei-Ou/git-inktrace/internal/gitlog"
	"github.com/ZhiWei-Ou/git-inktrace/internal/stats"
)

// inspectState is the detail page opened with enter: the commits touching a
// ranked file, or the files changed by a graph commit.
type inspectState struct {
	open    bool
	path    string              // file inspected from the ranking
	commit  *gitlog.GraphCommit // commit inspected from the graph
	commits []stats.FileCommit  // commits touching path
	body    string              // message of commit after its subject
	files   []gitlog.FileChange // files changed by commit
	loading bool
	err     error
	scroll  int
}

type inspectMsg struct {
	hash  string
	body  string
	files []gitlog.FileChange
	err   error
}

// inspectLines renders the scrolling part of the page: the commits touching
// the file, or the commit's message body followed by the files it changed.
func (m Model) inspectLines() []string {
	s := m.inspect
	var lines []string
	if s.commit == nil {
		for i := range s.commits {
			lines = append(lines, m.fileCommitLine(i))
		}
		return lines
	}
	if s.body != "" {
		for l := range strings.SplitSeq(ansi.Wrap(s.body, max(m.width-3, 1), ""), "\n") {
			lines = append(lines, " "+m.theme.text.Render(l))
		}
		lines = append(lines, "")
	}
	for i := range s.files {
		lines = append(lines, m.changedFileLine(i))
	}
	return lines
}

// openInspect opens the detail page of the selected file or commit.
func (m *Model) openInspect() tea.Cmd {
	switch m.view {
	case OwnersView:
		m.ownersEnter()
	case FilesView:
		if m.cursor >= len(m.files) {
			return nil
		}
		path := m.files[m.cursor].Path
		m.inspect = inspectState{open: true, path: path,
			commits: m.data.FileCommits(path, m.cal.Start, m.cal.End, m.author)}
	case GraphView:
		g := &m.graph
		if !g.isCommit(g.cursor) {
			return nil
		}
		c := g.lines[g.cursor].Commit
		m.inspect = inspectState{open: true, commit: c, loading: true}
		dir := m.opts.Dir
		return func() tea.Msg {
			files, err := gitlog.CommitFiles(dir, c.FullHash)
			if err != nil {
				return inspectMsg{hash: c.FullHash, err: err}
			}
			body, err := gitlog.CommitBody(dir, c.FullHash)
			return inspectMsg{hash: c.FullHash, body: body, files: files, err: err}
		}
	}
	return nil
}

func (m *Model) receiveInspect(msg inspectMsg) {
	s := &m.inspect
	if !s.open || s.commit == nil || s.commit.FullHash != msg.hash {
		return // closed, or another commit opened since
	}
	s.loading, s.body, s.files, s.err = false, msg.body, msg.files, msg.err
}

func (m *Model) inspectKey(key string) tea.Cmd {
	s := &m.inspect
	if key == "esc" || key == "q" {
		m.inspect = inspectState{}
		return nil
	}
	// The page only scrolls: the positions are its first visible rows.
	rows := m.inspectRows()
	s.scroll = moveCursor(key, s.scroll, max(len(m.inspectLines())-rows+1, 1), rows)
	return nil
}

// inspectRows is the height of the list: the screen without the header, tabs,
// title and the blank, status and help lines around the list.
func (m Model) inspectRows() int {
	return m.height - 6 - len(m.inspectTitle())
}

func (m Model) inspectTitle() []string {
	t := m.theme
	s := m.inspect
	if c := s.commit; c != nil {
		color := lipgloss.NewStyle().Foreground(t.authorColor(m.authorHue(c.Author, c.Email)))
		return []string{
			" " + t.dim.Render(c.Hash) + " " + t.text.Bold(true).Render(c.Subject),
			" " + color.Render(cellGlyph) + " " + t.text.Render(c.Author) + t.dim.Render(" <"+c.Email+"> · "+
				c.Time.Local().Format("2006-01-02 15:04")),
		}
	}
	return []string{" " + t.text.Bold(true).Render(truncatePath(s.path, max(m.width-2, 4)))}
}

func (m Model) inspectView() string {
	t := m.theme
	s := m.inspect
	var b strings.Builder
	for _, l := range m.inspectTitle() {
		b.WriteString(ansi.Truncate(l, max(m.width-1, 1), "…") + "\n")
	}
	b.WriteString("\n")
	switch {
	case s.err != nil:
		return b.String() + " " + t.dim.Render("Cannot read the commit: "+s.err.Error()) + "\n"
	case s.loading:
		return b.String() + " " + t.dim.Render("Loading changed files…") + "\n"
	}
	lines := m.inspectLines()
	if len(lines) == 0 {
		return b.String() + " " + t.dim.Render("No file changes") + "\n"
	}

	end := min(s.scroll+m.inspectRows(), len(lines))
	for _, line := range lines[s.scroll:end] {
		b.WriteString(ansi.Truncate(line, max(m.width-1, 1), "…") + "\n")
	}

	label := fmt.Sprintf("%d %s", len(s.commits), plural(len(s.commits), "commit", "commits"))
	if s.commit != nil {
		added, deleted := 0, 0
		for _, f := range s.files {
			added, deleted = added+max(f.Added, 0), deleted+max(f.Deleted, 0)
		}
		label = fmt.Sprintf("%d %s changed, +%d -%d", len(s.files), plural(len(s.files), "file", "files"), added, deleted)
	}
	b.WriteString(" " + t.accent.Render(label) + t.dim.Render(fmt.Sprintf(" · %d–%d of %d", s.scroll+1, end, len(lines))) + "\n")
	return b.String()
}

// fileCommitLine renders a commit touching the inspected file.
func (m Model) fileCommitLine(i int) string {
	t := m.theme
	c := m.inspect.commits[i]
	color := lipgloss.NewStyle().Foreground(t.authorColor(c.Author))
	name := ansi.Truncate(m.data.Authors[c.Author].Name, graphAuthorWidth-2, "…")
	return " " + t.dim.Render(c.Hash[:min(7, len(c.Hash))]) + "  " +
		t.dim.Render(c.Time.Format("2006-01-02")) + "  " +
		padRight(color.Render(cellGlyph)+" "+t.text.Render(name), graphAuthorWidth) + "  " +
		m.lineCounts(c.Added, c.Deleted) + "  " + t.text.Render(c.Subject)
}

// changedFileLine renders a file changed by the inspected commit with a bar
// of its added and deleted lines, like `git diff --stat`.
func (m Model) changedFileLine(i int) string {
	const barWidth = 20
	t := m.theme
	files := m.inspect.files
	f := files[i]
	most := 1
	for _, f := range files {
		most = max(most, f.Added+f.Deleted)
	}
	bar := ""
	if f.Added >= 0 {
		n := (f.Added + f.Deleted) * barWidth / most
		if n == 0 && f.Added+f.Deleted > 0 {
			n = 1
		}
		plus := (n*f.Added + max(f.Added+f.Deleted, 1) - 1) / max(f.Added+f.Deleted, 1)
		bar = addedStyle.Render(strings.Repeat("+", plus)) + deletedStyle.Render(strings.Repeat("-", n-plus))
	}
	path := t.text.Render(f.Path)
	if f.OldPath != "" {
		path = t.dim.Render(f.OldPath+" → ") + path
	}
	return " " + m.lineCounts(f.Added, f.Deleted) + "  " + padRight(bar, barWidth) + "  " + path
}

var (
	addedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950"))
	deletedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149"))
)

// lineCounts renders added and deleted line counts, or "bin" for binary files.
func (m Model) lineCounts(a, d int) string {
	if a < 0 {
		return padLeft(m.theme.dim.Render("bin"), 7) + "        "
	}
	return padLeft(addedStyle.Render("+"+human(a)), 7) + " " + padLeft(deletedStyle.Render("-"+human(d)), 7)
}

// authorHue is the color index of a commit author; authors outside the
// counted history share grey.
func (m Model) authorHue(name, email string) int {
	if id := m.data.AuthorOf(name, email); id >= 0 {
		return id
	}
	return len(authorHues)
}

// underline underlines a whole styled line, renewing the underline after
// each style reset so it also runs through the plain gaps.
func underline(s string) string {
	if lipgloss.ColorProfile() == termenv.Ascii {
		return s
	}
	return "\x1b[4m" + strings.ReplaceAll(s, "\x1b[0m", "\x1b[0;4m") + "\x1b[24m"
}
