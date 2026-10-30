package tui

import (
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ZhiWei-Ou/git-inktrace/internal/gitlog"
	"github.com/ZhiWei-Ou/git-inktrace/internal/stats"
)

// readFiles reads the file changes of commits; tests replace it.
var readFiles = gitlog.ReadFiles

// indexState tracks the background reading of each commit's file changes,
// which the heatmap, punch card and graph do not need, so they open on the
// commit headers alone while the file ranking waits for it.
type indexState struct {
	gen     int // bumped by each indexing, to drop older results
	loading bool
	done    *atomic.Int64 // commits read so far
	total   int           // commits to read
	err     error
	cache   map[string][]gitlog.FileChange // by hash; commits never change
}

type indexedMsg struct {
	gen   int
	data  *stats.Data
	cache map[string][]gitlog.FileChange
	err   error
}

type indexTickMsg struct{ gen int }

type dataMsg struct {
	commits []gitlog.Commit
	tracked []string
	err     error
}

// prepareIndex starts a new indexing of the commits not read before.
func (m *Model) prepareIndex() {
	s := &m.index
	s.gen++
	s.loading, s.err, s.done, s.total = true, nil, &atomic.Int64{}, 0
	for _, c := range m.commits {
		if _, ok := s.cache[c.Hash]; !ok {
			s.total++
		}
	}
}

// indexCmd reads the file changes prepared by prepareIndex in parallel git
// processes and builds the full statistics, ticking the progress meanwhile.
func (m Model) indexCmd() tea.Cmd {
	s := m.index
	dir, commits, tracked, gen := m.opts.Dir, m.commits, m.tracked, s.gen
	read := func() tea.Msg {
		var todo []gitlog.Commit
		for _, c := range commits {
			if _, ok := s.cache[c.Hash]; !ok {
				todo = append(todo, c)
			}
		}
		cache := make(map[string][]gitlog.FileChange, len(commits))
		if len(todo) > 0 {
			files, err := readFiles(dir, todo, runtime.NumCPU(), s.done)
			if err != nil {
				return indexedMsg{gen: gen, err: err}
			}
			for i, c := range todo {
				cache[c.Hash] = files[i]
			}
		}
		for _, c := range commits {
			if _, ok := cache[c.Hash]; !ok {
				cache[c.Hash] = s.cache[c.Hash]
			}
		}
		return indexedMsg{gen: gen, data: stats.New(withFiles(commits, cache), tracked), cache: cache}
	}
	return tea.Batch(read, indexTick(gen))
}

func indexTick(gen int) tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return indexTickMsg{gen: gen} })
}

// receiveIndexTick keeps ticking, so the progress redraws, until the
// indexing ends.
func (m *Model) receiveIndexTick(msg indexTickMsg) tea.Cmd {
	if msg.gen != m.index.gen || !m.index.loading {
		return nil
	}
	return indexTick(msg.gen)
}

func (m *Model) receiveIndexed(msg indexedMsg) {
	s := &m.index
	if msg.gen != s.gen {
		return // the history was reloaded since
	}
	s.loading = false
	if msg.err != nil {
		s.err = msg.err
		return
	}
	s.cache = msg.cache
	m.data = msg.data
	m.refresh()
}

// withFiles returns commits with the file changes known for them.
func withFiles(commits []gitlog.Commit, cache map[string][]gitlog.FileChange) []gitlog.Commit {
	full := make([]gitlog.Commit, len(commits))
	for i, c := range commits {
		full[i] = c
		full[i].Files = cache[c.Hash]
	}
	return full
}

// reloadData reads the commit headers again after history changed.
func (m Model) reloadData() tea.Cmd {
	dir, all := m.opts.Dir, m.opts.All
	return func() tea.Msg {
		commits, err := gitlog.ReadHeaders(dir, gitlog.Options{All: all})
		if err != nil {
			return dataMsg{err: err}
		}
		tracked, err := gitlog.Tracked(dir)
		return dataMsg{commits: commits, tracked: tracked, err: err}
	}
}

// receiveData shows the reloaded history at once, with the file changes
// already known, and indexes the commits that are new.
func (m *Model) receiveData(msg dataMsg) tea.Cmd {
	if msg.err != nil {
		m.graph.status = "Cannot reload history: " + msg.err.Error()
		return nil
	}
	m.commits, m.tracked = msg.commits, msg.tracked
	m.data = stats.New(withFiles(m.commits, m.index.cache), m.tracked)
	m.refresh()
	m.prepareIndex()
	return m.indexCmd()
}

// indexStatus describes the indexing for the file ranking, or is empty
// when it is done.
func (m Model) indexStatus() string {
	t := m.theme
	s := m.index
	switch {
	case s.err != nil:
		return t.dim.Render("Cannot read changed files: " + s.err.Error())
	case !s.loading:
		return ""
	}
	const barWidth = 24
	done := min(int(s.done.Load()), s.total)
	filled := 0
	if s.total > 0 {
		filled = done * barWidth / s.total
	}
	return t.text.Render("Reading changed files ") +
		t.accent.Render(strings.Repeat("█", filled)) + t.dim.Render(strings.Repeat("░", barWidth-filled)) +
		t.dim.Render(fmt.Sprintf(" %s / %s commits", thousands(done), thousands(s.total)))
}
