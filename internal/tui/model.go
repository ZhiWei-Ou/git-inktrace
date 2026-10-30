// Package tui renders the interactive heatmap, file ranking and branch graph.
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ZhiWei-Ou/git-inktrace/internal/stats"
)

type view int

const (
	heatmapView view = iota
	filesView
	graphView
	viewCount
)

const maxWeeks = 53

// Options configures the TUI.
type Options struct {
	Repo   string // repository name shown in the title
	Branch string
	Top    int    // number of files in the ranking
	Dir    string // repository root, read by the graph view
	All    bool   // graph every branch initially, not only HEAD
}

// Model is the bubbletea model.
type Model struct {
	data  *stats.Data
	opts  Options
	theme theme

	width, height int
	view          view
	author        int       // stats.AllAuthors or an author index
	end           time.Time // last day of the calendar

	cal stats.Calendar
	// Commits per author, ignoring the author filter: in the calendar month
	// of the window's last day, and in the whole window.
	monthShares, windowShares []int
	files                     []stats.FileStat
	scroll                    int // first visible row of the file ranking

	graph graphState
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
		author: stats.AllAuthors,
		end:    defaultEnd(data),
		graph:  graphState{all: opts.All, limit: graphPage},
	}
	// Render a valid first frame before the terminal size arrives.
	m.refresh()
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

func (m Model) Init() tea.Cmd { return nil }

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
	}
	return m, nil
}

func (m *Model) setView(v view) tea.Cmd {
	m.view = v
	if v == graphView {
		return m.loadGraph()
	}
	return nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "tab":
		return m, m.setView((m.view + 1) % viewCount)
	case "shift+tab":
		return m, m.setView((m.view + viewCount - 1) % viewCount)
	case "1":
		return m, m.setView(heatmapView)
	case "2":
		return m, m.setView(filesView)
	case "3":
		return m, m.setView(graphView)
	case "a":
		m.cycleAuthor(1)
	case "A":
		m.cycleAuthor(-1)
	case "left", "h", "right", "l", "t":
		if m.view == graphView {
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
		case filesView:
			m.scrollFiles(msg.String())
		case graphView:
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
	page := max(m.fileRows(), 1)
	switch key {
	case "up", "k":
		m.scroll--
	case "down", "j":
		m.scroll++
	case "pgup", "ctrl+u":
		m.scroll -= page
	case "pgdown", "ctrl+d", " ":
		m.scroll += page
	case "home", "g":
		m.scroll = 0
	case "end", "G":
		m.scroll = len(m.files)
	}
	m.clampScroll()
}

func (m *Model) clampScroll() {
	m.scroll = min(m.scroll, len(m.files)-m.fileRows())
	m.scroll = max(m.scroll, 0)
}

// refresh recomputes the calendar and ranking for the current window and filter.
func (m *Model) refresh() {
	weeks := min(max((m.width-gutter)/cellWidth, 1), maxWeeks)
	m.cal = m.data.Calendar(m.end, weeks, m.author)
	monthStart := time.Date(m.cal.End.Year(), m.cal.End.Month(), 1, 0, 0, 0, 0, time.Local)
	m.monthShares = m.data.AuthorCommits(monthStart, m.cal.End)
	m.windowShares = m.data.AuthorCommits(m.cal.Start, m.cal.End)
	m.files = m.data.FileRank(m.cal.Start, m.cal.End, m.author, m.opts.Top)
	m.clampScroll()
}
