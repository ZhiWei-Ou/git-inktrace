// Package tui renders the interactive heatmap and file ranking.
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
)

const maxWeeks = 53

// Options configures the TUI.
type Options struct {
	Repo   string // repository name shown in the title
	Branch string
	Top    int // number of files in the ranking
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
	cursor        time.Time // selected day

	cal    stats.Calendar
	files  []stats.FileStat
	scroll int // first visible row of the file ranking
}

// New creates a model showing the year up to today, or up to the latest
// commit when the repository has been idle for longer than that.
func New(data *stats.Data, opts Options) Model {
	end := defaultEnd(data)
	return Model{
		data:   data,
		opts:   opts,
		theme:  newTheme(lipgloss.HasDarkBackground()),
		width:  80,
		height: 24,
		author: stats.AllAuthors,
		end:    end,
		cursor: end,
	}
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

// daysBetween counts calendar days from a to b, independent of DST shifts.
func daysBetween(a, b time.Time) int {
	ua := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	ub := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(ub.Sub(ua).Hours() / 24)
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
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "tab", "shift+tab":
		m.view = 1 - m.view
		return m, nil
	case "1":
		m.view = heatmapView
		return m, nil
	case "2":
		m.view = filesView
		return m, nil
	case "a":
		m.cycleAuthor(1)
	case "A":
		m.cycleAuthor(-1)
	case "[":
		m.shift(-len(m.cal.Weeks) * 7)
	case "]":
		m.shift(len(m.cal.Weeks) * 7)
	case "t":
		m.end = defaultEnd(m.data)
		m.cursor = m.end
	default:
		if m.view == heatmapView {
			m.moveCursor(msg.String())
		} else {
			m.scrollFiles(msg.String())
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

// shift moves the calendar window by days, never past today.
func (m *Model) shift(days int) {
	end := m.end.AddDate(0, 0, days)
	if t := today(); end.After(t) {
		end = t
	}
	m.cursor = m.cursor.AddDate(0, 0, daysBetween(m.end, end))
	m.end = end
}

func (m *Model) moveCursor(key string) {
	days := 0
	switch key {
	case "left", "h":
		days = -7
	case "right", "l":
		days = 7
	case "up", "k":
		days = -1
	case "down", "j":
		days = 1
	case "home", "g":
		m.cursor = m.cal.Start
		return
	case "end", "G":
		m.cursor = m.cal.End
		return
	}
	next := m.cursor.AddDate(0, 0, days)
	if !next.Before(m.cal.Start) && !next.After(m.cal.End) {
		m.cursor = next
	}
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
	if m.cursor.IsZero() || m.cursor.Before(m.cal.Start) || m.cursor.After(m.cal.End) {
		m.cursor = m.cal.End
	}
	m.files = m.data.FileRank(m.cal.Start, m.cal.End, m.author, m.opts.Top)
	m.clampScroll()
}
