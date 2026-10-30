// Package tui renders the interactive heatmap, file ranking, branch graph
// and punch card.
package tui

import (
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ZhiWei-Ou/git-inktrace/internal/gitlog"
	"github.com/ZhiWei-Ou/git-inktrace/internal/stats"
)

// View is a page of the TUI.
type View int

const (
	HeatmapView View = iota
	FilesView
	GraphView
	HoursView
	OwnersView
)

const maxWeeks = 53

// Options configures the TUI.
type Options struct {
	Repo   string // repository name shown in the title
	Branch string
	Top    int    // number of files in the ranking
	Dir    string // repository root, read by the graph view
	All    bool   // history covers every branch, remote branch and tag
	// Created is the date of the repository's first commit on any ref; the
	// oldest counted commit stands in when it is zero.
	Created time.Time
	View    View // page shown first
	// Commits are the headers behind the data, whose file changes load in
	// the background; nil when the data already has them.
	Commits  []gitlog.Commit
	Tracked  []string // paths in HEAD, for the reloaded ranking
	Branches int      // local and remote branches; many start the graph on the current one
	// Authors keeps only the authors matching one of these patterns, as
	// stats.Data.Filter does, and hides the pages about the whole team.
	Authors []string
}

// allBranchesLimit is the most branches the graph shows all of at first.
const allBranchesLimit = 10

// Model is the bubbletea model.
type Model struct {
	data  *stats.Data
	opts  Options
	theme theme

	width, height int
	view          View
	end           time.Time // last day of the calendar

	cal stats.Calendar
	// Commits per author in the calendar month of the window's last day,
	// and in the whole window.
	monthShares, windowShares []int
	punch                     stats.PunchCard
	files                     []stats.FileStat
	scroll                    int // first visible row of the file ranking
	cursor                    int // selected row of the file ranking

	graph    graphState
	owners   ownersState
	inspect  inspectState // details of the selected file or commit
	helpOpen bool         // the help page covers the current page

	commits []gitlog.Commit // headers indexed for file changes
	tracked []string
	index   indexState
}

// New creates a model showing the year up to today, or up to the latest
// commit when the repository has been idle for longer than that.
func New(data *stats.Data, opts Options) Model {
	m := Model{
		data:   data.Filter(opts.Authors),
		opts:   opts,
		theme:  newTheme(lipgloss.HasDarkBackground()),
		width:  80,
		height: 24,
		view:   opts.View,
		graph:  graphState{all: opts.Branches <= allBranchesLimit, limit: graphPage},
	}
	m.end = defaultEnd(m.data)
	if len(opts.Commits) > 0 {
		m.commits, m.tracked = opts.Commits, opts.Tracked
		m.prepareIndex()
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
	var cmds []tea.Cmd
	if m.graph.loading {
		cmds = append(cmds, m.fetchGraph())
	}
	if m.index.loading {
		cmds = append(cmds, m.indexCmd())
	}
	return tea.Batch(cmds...)
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
	case rewordCheckMsg:
		return m, m.receiveRewordCheck(msg)
	case rewordedMsg:
		return m, m.receiveReworded(msg)
	case dataMsg:
		return m, m.receiveData(msg)
	case indexedMsg:
		m.receiveIndexed(msg)
	case indexTickMsg:
		return m, m.receiveIndexTick(msg)
	}
	return m, nil
}

// filtered reports whether the history is narrowed to some authors.
func (m Model) filtered() bool { return len(m.opts.Authors) > 0 }

// views lists the pages in tab order; the author filter leaves out the
// hours and owners, which compare the whole team.
func (m Model) views() []View {
	if m.filtered() {
		return []View{HeatmapView, FilesView, GraphView}
	}
	return []View{HeatmapView, FilesView, GraphView, HoursView, OwnersView}
}

// stepView moves n pages along the tabs, wrapping around.
func (m *Model) stepView(n int) tea.Cmd {
	vs := m.views()
	i := slices.Index(vs, m.view)
	return m.setView(vs[(i+n+len(vs))%len(vs)])
}

func (m *Model) setView(v View) tea.Cmd {
	m.view = v
	switch v {
	case GraphView:
		return m.loadGraph()
	case OwnersView:
		m.refreshOwners()
	}
	return nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.view == GraphView && m.graph.typing && msg.String() != "ctrl+c" {
		return m, m.searchKey(msg)
	}
	if m.view == GraphView && m.graph.confirm != nil && msg.String() != "ctrl+c" {
		return m, m.confirmKey(msg.String())
	}
	if m.helpOpen {
		if m.helpKey(msg.String()) {
			return m, tea.Quit
		}
		return m, nil
	}
	if msg.String() == "?" {
		m.helpOpen = true
		return m, nil
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
		case "ctrl+c", "tab", "shift+tab", "1", "2", "3", "4", "5":
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
		return m, m.stepView(1)
	case "shift+tab":
		return m, m.stepView(-1)
	case "1", "2", "3", "4", "5":
		if i, vs := int(msg.String()[0]-'1'), m.views(); i < len(vs) {
			return m, m.setView(vs[i])
		}
		return m, nil
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
		case OwnersView:
			m.ownersKey(msg.String())
		}
		return m, nil
	}
	m.refresh()
	return m, nil
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
// window.
func (m *Model) refresh() {
	weeks := min(max((m.width-gutter)/cellWidth, 1), maxWeeks)
	m.cal = m.data.Calendar(m.end, weeks)
	monthStart := time.Date(m.cal.End.Year(), m.cal.End.Month(), 1, 0, 0, 0, 0, time.Local)
	m.monthShares = m.data.AuthorCommits(monthStart, m.cal.End)
	m.windowShares = m.data.AuthorCommits(m.cal.Start, m.cal.End)
	m.punch = m.data.PunchCard(m.cal.Start, m.cal.End)
	m.files = m.data.FileRank(m.cal.Start, m.cal.End, m.opts.Top)
	m.clampScroll()
	m.refreshOwners()
}
