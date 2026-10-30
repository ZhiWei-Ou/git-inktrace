// Package tui renders the interactive heatmap, file ranking, branch graph
// and punch card.
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ZhiWei-Ou/git-inktrace/internal/stats"
)

// View is a page of the TUI.
type View int

const (
	HeatmapView View = iota
	FilesView
	GraphView
	HoursView
	viewCount
)

const maxWeeks = 53

// Options configures the TUI.
type Options struct {
	Repo   string // repository name shown in the title
	Branch string
	Top    int    // number of files in the ranking
	Dir    string // repository root, read by the graph view
	View   View   // page shown first
}

// Model is the bubbletea model.
type Model struct {
	data  *stats.Data
	opts  Options
	theme theme

	width, height int
	view          View
	author        int       // stats.AllAuthors or an author index
	end           time.Time // last day of the calendar

	cal stats.Calendar
	// Commits per author, ignoring the author filter: in the calendar month
	// of the window's last day, and in the whole window.
	monthShares, windowShares []int
	punch                     stats.PunchCard
	files                     []stats.FileStat
	scroll                    int // first visible row of the file ranking
	cursor                    int // selected row of the file ranking

	graph   graphState
	inspect inspectState // details of the selected file or commit
}

// New creates a model showing the year up to today, or up to the latest
// commit when the repository has been idle for longer than that.
func New(data *stats.Data, opts Options) Model {
	m := Model{
		data:   data,
		opts:   opts,
		theme:  newTheme(lipgloss.HasDarkBackground()),
		width:  80,
		height: 24,
		view:   opts.View,
		author: stats.AllAuthors,
		end:    defaultEnd(data),
		graph:  graphState{all: true, limit: graphPage},
	}
	// Render a valid first frame before the terminal size arrives.
	m.refresh()
	if m.view == GraphView {
		m.graph.loading = true // Init fetches the first page
	}
	return m
}

func defaultEnd(data *stats.Data) time.Time {
	end := today()
	if latest := data.Latest(); !latest.IsZero() && latest.Before(end.AddDate(-1, 0, 0)) {
		end = midnight(latest)
	}
	return end
}

func today() time.Time { return midnight(time.Now()) }

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

// Run starts the TUI in the alternate screen.
func Run(data *stats.Data, opts Options) error {
	_, err := tea.NewProgram(New(data, opts), tea.WithAltScreen()).Run()
	return err
}

func (m Model) Init() tea.Cmd {
	if m.graph.loading {
		return m.fetchGraph()
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.refresh()
	case tea.KeyMsg:
		return m.handleKey(msg)
	case graphMsg:
		return m, m.receiveGraph(msg)
	case copiedMsg:
		m.receiveCopied(msg)
	case inspectMsg:
		m.receiveInspect(msg)
	}
	return m, nil
}

func (m *Model) setView(v View) tea.Cmd {
	m.view = v
	if v == GraphView {
		return m.loadGraph()
	}
	return nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.view == GraphView && m.graph.typing && msg.String() != "ctrl+c" {
		return m, m.searchKey(msg)
	}
	// Fast typing or key repeat can deliver several runes in one message.
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste {
		var cmds []tea.Cmd
		var model tea.Model = m
		for _, r := range msg.Runes {
			var cmd tea.Cmd
			model, cmd = model.(Model).handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt})
			cmds = append(cmds, cmd)
		}
		return model, tea.Batch(cmds...)
	}
	if m.inspect.open {
		switch msg.String() {
		case "ctrl+c", "tab", "shift+tab", "1", "2", "3", "4":
			m.inspect = inspectState{} // leave it for the view keys below
		default:
			return m, m.inspectKey(msg.String())
		}
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter":
		return m, m.openInspect()
	case "tab":
		return m, m.setView((m.view + 1) % viewCount)
	case "shift+tab":
		return m, m.setView((m.view + viewCount - 1) % viewCount)
	case "1":
		return m, m.setView(HeatmapView)
	case "2":
		return m, m.setView(FilesView)
	case "3":
		return m, m.setView(GraphView)
	case "4":
		return m, m.setView(HoursView)
	case "a", "A":
		if m.view == GraphView {
			return m, m.graphKey(msg.String()) // a switches the graph's branches
		}
		if msg.String() == "a" {
			m.cycleAuthor(1)
		} else {
			m.cycleAuthor(-1)
		}
	case "left", "h", "right", "l", "t":
		if m.view == GraphView {
			return m, nil // the graph has no time window
		}
		switch msg.String() {
		case "left", "h":
			m.shiftYears(-1)
		case "right", "l":
			m.shiftYears(1)
		default:
			m.end = defaultEnd(m.data)
		}
	default:
		switch m.view {
		case FilesView:
			m.scrollFiles(msg.String())
		case GraphView:
			return m, m.graphKey(msg.String())
		}
		return m, nil
	}
	m.refresh()
	return m, nil
}

func (m *Model) cycleAuthor(step int) {
	n := len(m.data.Authors)
	if n == 0 {
		return
	}
	// Positions 0..n-1 are authors and n is "all".
	pos := m.author
	if pos == stats.AllAuthors {
		pos = n
	}
	pos = (pos + step + n + 1) % (n + 1)
	if pos == n {
		pos = stats.AllAuthors
	}
	m.author = pos
}

// shiftYears moves the calendar window by whole years, or by whole windows
// when the terminal shows less than a year, so no dates are skipped. It moves
// back only while older commits lie before the window, and never past the
// default end.
func (m *Model) shiftYears(n int) {
	if n < 0 && !m.hasEarlier() {
		return
	}
	end := m.end.AddDate(n, 0, 0)
	if days := len(m.cal.Weeks) * 7; days < 365 {
		end = m.end.AddDate(0, 0, n*days)
	}
	if latest := defaultEnd(m.data); end.After(latest) {
		end = latest
	}
	m.end = end
}

// hasEarlier reports whether commits exist before the window.
func (m Model) hasEarlier() bool {
	return midnight(m.data.Earliest()).Before(m.cal.Start)
}

func (m *Model) scrollFiles(key string) {
	m.cursor = moveCursor(key, m.cursor, len(m.files), m.fileRows())
	m.clampScroll()
}

func (m *Model) clampScroll() {
	m.cursor = max(min(m.cursor, len(m.files)-1), 0)
	m.scroll = follow(m.cursor, m.scroll, m.fileRows(), len(m.files))
}

// moveCursor applies a list navigation key to the selected row of n rows,
// page rows at a time for the page keys.
func moveCursor(key string, cursor, n, page int) int {
	page = max(page, 1)
	switch key {
	case "up", "k":
		cursor--
	case "down", "j":
		cursor++
	case "pgup", "ctrl+u":
		cursor -= page
	case "pgdown", "ctrl+d", " ":
		cursor += page
	case "home", "g":
		cursor = 0
	case "end", "G":
		cursor = n - 1
	}
	return max(min(cursor, n-1), 0)
}

// follow returns the first visible row of a list of n rows, rows at a time,
// so that the cursor stays in view.
func follow(cursor, scroll, rows, n int) int {
	rows = max(rows, 1)
	if cursor < scroll {
		scroll = cursor
	}
	if cursor >= scroll+rows {
		scroll = cursor - rows + 1
	}
	return max(min(scroll, n-rows), 0)
}

// refresh recomputes the calendar, punch card and ranking for the current
// window and filter.
func (m *Model) refresh() {
	weeks := min(max((m.width-gutter)/cellWidth, 1), maxWeeks)
	m.cal = m.data.Calendar(m.end, weeks, m.author)
	monthStart := time.Date(m.cal.End.Year(), m.cal.End.Month(), 1, 0, 0, 0, 0, time.Local)
	m.monthShares = m.data.AuthorCommits(monthStart, m.cal.End)
	m.windowShares = m.data.AuthorCommits(m.cal.Start, m.cal.End)
	m.punch = m.data.PunchCard(m.cal.Start, m.cal.End, m.author)
	m.files = m.data.FileRank(m.cal.Start, m.cal.End, m.author, m.opts.Top)
	m.clampScroll()
}
