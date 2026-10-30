package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ZhiWei-Ou/git-inktrace/internal/gitlog"
	"github.com/ZhiWei-Ou/git-inktrace/internal/stats"
)

func testModel(t *testing.T, width int) Model {
	t.Helper()
	now := time.Now()
	var commits []gitlog.Commit
	for i := 0; i < 40; i++ {
		name := []string{"Alice", "Bob", "Carol"}[i%3]
		commits = append(commits, gitlog.Commit{
			Author: name, Email: strings.ToLower(name) + "@example.com",
			Time:  now.AddDate(0, 0, -i*3),
			Files: []gitlog.FileChange{{Path: "dir/file" + string(rune('a'+i%5)) + ".go", Added: i, Deleted: 1}},
		})
	}
	m := New(stats.New(commits, nil), Options{Repo: "demo", Branch: "main", Top: 100})
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	return next.(Model)
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func TestHeatmapFitsWidth(t *testing.T) {
	for _, width := range []int{40, 80, 120, 200} {
		m := testModel(t, width)
		want := min((width-gutter)/cellWidth, maxWeeks)
		if len(m.cal.Weeks) != want {
			t.Errorf("width %d: %d weeks, want %d", width, len(m.cal.Weeks), want)
		}
		if m.cal.Total == 0 || !strings.Contains(m.View(), "Alice") {
			t.Errorf("width %d: heatmap missing commits", width)
		}
	}
}

func TestYearNavigation(t *testing.T) {
	now := time.Now()
	var commits []gitlog.Commit
	for _, years := range []int{0, 1, 2} {
		commits = append(commits, gitlog.Commit{Author: "Alice", Email: "alice@example.com", Time: now.AddDate(-years, 0, 0)})
	}
	m := New(stats.New(commits, nil), Options{Top: 10})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(Model)
	end := m.cal.End

	m = press(m, "right")
	if !m.cal.End.Equal(end) {
		t.Errorf("moved past the latest window: %v", m.cal.End)
	}
	// A 53-week window ending a year ago also shows the commit from two
	// years ago, so a second step back is refused.
	m = press(m, "left", "h")
	if want := end.AddDate(-1, 0, 0); !m.cal.End.Equal(want) || m.cal.Total != 2 {
		t.Errorf("after stepping back: end %v (want %v), total %d", m.cal.End, want, m.cal.Total)
	}
	m = press(m, "l")
	if !m.cal.End.Equal(end) {
		t.Errorf("forward: end %v, want %v", m.cal.End, end)
	}
	m = press(m, "left", "t")
	if !m.cal.End.Equal(end) {
		t.Errorf("t did not return to the latest window: %v", m.cal.End)
	}
}

func TestAuthorFilter(t *testing.T) {
	m := testModel(t, 120)
	m = press(m, "a")
	if m.author != 0 || m.cal.Total >= 40 {
		t.Errorf("author filter: author %d, total %d", m.author, m.cal.Total)
	}
	m = press(m, "A", "A")
	if m.author != 2 {
		t.Errorf("reverse cycle: author %d, want 2", m.author)
	}
	m = press(m, "a")
	if m.author != stats.AllAuthors {
		t.Errorf("cycle back: author %d", m.author)
	}
}

func TestFilesView(t *testing.T) {
	m := press(testModel(t, 100), "tab")
	if m.view != FilesView || len(m.files) != 5 {
		t.Fatalf("view %d, %d files", m.view, len(m.files))
	}
	out := m.View()
	if !strings.Contains(out, "dir/filea.go") || !strings.Contains(out, "top author") {
		t.Errorf("file table missing content:\n%s", out)
	}
	m = press(m, "G")
	if m.scroll != 0 {
		t.Errorf("scrolled %d with all rows visible", m.scroll)
	}
}

func TestShareColumns(t *testing.T) {
	now := time.Now()
	commits := []gitlog.Commit{
		{Author: "Alice", Email: "alice@example.com", Time: now},
		{Author: "Bob", Email: "bob@example.com", Time: now.AddDate(-3, 0, 0)},
		{Author: "Bob", Email: "bob@example.com", Time: now.AddDate(-3, 0, 1)},
	}
	m := New(stats.New(commits, nil), Options{Top: 10})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	out := next.(Model).View()

	for _, want := range []string{"this window", "all history", "Alice <alice@example.com>", "100.0%", "33.3%", "66.7%"} {
		if !strings.Contains(out, want) {
			t.Errorf("legend missing %q:\n%s", want, out)
		}
	}
}

func TestShareColumnsSingleWindow(t *testing.T) {
	now := time.Now()
	commits := []gitlog.Commit{
		{Author: "Alice", Email: "alice@example.com", Time: now},
		{Author: "Bob", Email: "bob@example.com", Time: now.AddDate(0, -2, 0)},
	}
	m := New(stats.New(commits, nil), Options{Top: 10})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	out := next.(Model).View()

	if strings.Contains(out, "this window") || !strings.Contains(out, "all history") || !strings.Contains(out, "50.0%") {
		t.Errorf("want only the all-history column:\n%s", out)
	}
}

func TestShareColumnsMonth(t *testing.T) {
	now := time.Now()
	month := now.Format("Jan 2006")
	commits := []gitlog.Commit{
		{Author: "Alice", Email: "alice@example.com", Time: now},
		{Author: "Bob", Email: "bob@example.com", Time: now.AddDate(0, -2, 0)},
		{Author: "Bob", Email: "bob@example.com", Time: now.AddDate(-3, 0, 0)},
	}
	m := New(stats.New(commits, nil), Options{Top: 10})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	out := next.(Model).View()

	for _, want := range []string{month, "this window", "all history", "100.0%", "50.0%", "66.7%"} {
		if !strings.Contains(out, want) {
			t.Errorf("legend missing %q:\n%s", want, out)
		}
	}
	// A month column identical to the window column (both empty here) is dropped.
	m = press(next.(Model), "left")
	if month := m.cal.End.Format("Jan 2006"); strings.Contains(m.View(), month) {
		t.Errorf("month column shown for a past window:\n%s", m.View())
	}
}

func TestViewBeforeWindowSize(t *testing.T) {
	m := New(stats.New([]gitlog.Commit{{Author: "Alice", Email: "a@x", Time: time.Now()}}, nil), Options{})
	if !strings.Contains(m.View(), "Alice") {
		t.Error("first frame missing content")
	}
}

func TestNarrowWindowReachesOlderCommits(t *testing.T) {
	now := time.Now()
	commits := []gitlog.Commit{
		{Author: "Alice", Email: "alice@example.com", Time: now},
		{Author: "Alice", Email: "alice@example.com", Time: now.AddDate(0, -10, 0)},
	}
	m := New(stats.New(commits, nil), Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30}) // about half a year
	m = next.(Model)
	if m.cal.Total != 1 {
		t.Fatalf("narrow window total = %d, want 1", m.cal.Total)
	}
	m = press(m, "left")
	if m.cal.Total != 1 || !m.cal.End.Before(now.AddDate(0, -6, 0)) {
		t.Errorf("could not move back to the older commit: %v..%v total %d", m.cal.Start, m.cal.End, m.cal.Total)
	}
}

func TestFilesViewMarksDeleted(t *testing.T) {
	commits := []gitlog.Commit{{Author: "Alice", Email: "a@x", Time: time.Now(), Files: []gitlog.FileChange{
		{Path: "kept.go", Added: 1}, {Path: "removed.go", Added: 2},
	}}}
	m := New(stats.New(commits, []string{"kept.go"}), Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	out := press(next.(Model), "tab").View()
	if !strings.Contains(out, "removed.go") || strings.Count(out, "(deleted)") != 1 {
		t.Errorf("want only removed.go tagged as deleted:\n%s", out)
	}
}

func TestGraphView(t *testing.T) {
	now := time.Now()
	commits := []gitlog.Commit{
		{Author: "Alice", Email: "alice@example.com", Time: now},
		{Author: "Bob", Email: "bob@example.com", Time: now},
	}
	m := New(stats.New(commits, nil), Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	m = next.(Model)

	m = press(m, "3")
	if m.view != GraphView || !m.graph.loading {
		t.Fatalf("graph not loading: view %d, %+v", m.view, m.graph)
	}
	if !strings.Contains(m.View(), "Loading graph") {
		t.Error("missing loading state")
	}

	var lines []gitlog.GraphLine
	for i := 0; i < 20; i++ {
		lines = append(lines, gitlog.GraphLine{Graph: "*", Commit: &gitlog.GraphCommit{
			Hash: fmt.Sprintf("%07d", i), FullHash: fmt.Sprintf("full-%07d", i), Subject: fmt.Sprintf("change %d", i),
			Author: "Bob", Email: "bob@example.com", Time: now,
			Refs: []gitlog.Ref{{Name: "HEAD -> main", Kind: gitlog.RefHead}},
		}}, gitlog.GraphLine{Graph: "|"})
	}
	// Fewer commits than requested means the whole history is loaded.
	if !m.graph.all {
		t.Error("graph does not start with all branches")
	}
	next, cmd := m.Update(graphMsg{all: true, limit: graphPage, lines: lines})
	m = next.(Model)
	if cmd != nil || !m.graph.done {
		t.Fatalf("20 of %d commits: cmd %v, done %v", graphPage, cmd != nil, m.graph.done)
	}
	out := m.View()
	for _, want := range []string{"change 0", "HEAD -> main", "Bob", "0000000"} {
		if !strings.Contains(out, want) {
			t.Errorf("graph missing %q:\n%s", want, out)
		}
	}

	// The cursor walks commit lines only and the view follows it.
	if m.graph.cursor != 0 {
		t.Errorf("initial cursor %d", m.graph.cursor)
	}
	m = press(m, "j")
	if m.graph.cursor != 2 {
		t.Errorf("j moved to line %d, want the next commit at 2", m.graph.cursor)
	}
	// Runes batched into one message by fast typing count one by one.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj")})
	if m = next.(Model); m.graph.cursor != 6 {
		t.Errorf("batched jj moved to %d, want 6", m.graph.cursor)
	}
	m = press(m, "k", "k", "k", "k")
	if m.graph.cursor != 0 {
		t.Errorf("k past the top: %d", m.graph.cursor)
	}
	m = press(m, "G")
	if last := len(lines) - 2; m.graph.cursor != last || m.graph.scroll+m.graphRows() <= last {
		t.Errorf("G: cursor %d, scroll %d", m.graph.cursor, m.graph.scroll)
	}
	// Underlines only render with a color profile, absent without a terminal.
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	selected := m.graphLine(lines[m.graph.cursor], true)
	if !strings.Contains(selected, "\x1b[4") || strings.Contains(m.graphLine(lines[0], false), "\x1b[4") {
		t.Errorf("only the selected line should be underlined: %q", selected)
	}

	// Enter copies the selected commit's full hash.
	var copied string
	copyToClipboard = func(s string) error { copied = s; return nil }
	defer func() { copyToClipboard = clipboard.WriteAll }()
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter produced no command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if copied != "full-0000019" || !strings.Contains(m.View(), "Copied full-0000019") {
		t.Errorf("copied %q", copied)
	}
	copyToClipboard = func(string) error { return errors.New("no clipboard utility") }
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	if !strings.Contains(next.(Model).View(), "Copy failed: no clipboard utility") {
		t.Error("copy failure not reported")
	}

	// Switching scope reloads from scratch, and stale results are dropped.
	m = press(m, "a")
	if m.graph.all || !m.graph.loading || len(m.graph.lines) != 0 || m.author != stats.AllAuthors {
		t.Errorf("a did not switch to the current branch: %+v, author %d", m.graph, m.author)
	}
	next, _ = m.Update(graphMsg{all: true, limit: graphPage, lines: lines})
	if next.(Model).graph.loaded {
		t.Error("stale graph result accepted")
	}

	// The time window keys do nothing here.
	end := m.end
	if m = press(m, "left"); !m.end.Equal(end) {
		t.Error("left moved the heatmap window from the graph view")
	}
}

func TestGraphLoadsMoreNearEnd(t *testing.T) {
	m := New(stats.New([]gitlog.Commit{{Author: "A", Email: "a@x", Time: time.Now()}}, nil), Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = press(next.(Model), "3")
	lines := make([]gitlog.GraphLine, graphPage)
	for i := range lines {
		lines[i] = gitlog.GraphLine{Graph: "*", Commit: &gitlog.GraphCommit{Hash: "h", Author: "A", Email: "a@x"}}
	}
	next, _ = m.Update(graphMsg{all: true, limit: graphPage, lines: lines})
	m = press(next.(Model), "G")
	if !m.graph.loading || m.graph.limit != 2*graphPage {
		t.Errorf("no further page requested: %+v", m.graph.limit)
	}
}

func TestStartView(t *testing.T) {
	data := stats.New([]gitlog.Commit{{Author: "Alice", Email: "a@x", Time: time.Now()}}, nil)
	if m := New(data, Options{}); m.view != HeatmapView || m.Init() != nil {
		t.Errorf("default start: view %d", m.view)
	}
	if m := New(data, Options{View: FilesView}); m.view != FilesView || m.Init() != nil {
		t.Errorf("files start: view %d", m.view)
	}
	// Starting on the graph loads its first page once.
	m := New(data, Options{View: GraphView})
	if m.view != GraphView || m.Init() == nil {
		t.Fatalf("graph start: view %d, no load", m.view)
	}
	if m = press(m, "3"); !m.graph.loading || m.setView(GraphView) != nil {
		t.Error("graph start loaded twice")
	}
}

func TestGraphSearch(t *testing.T) {
	now := time.Now()
	commit := func(hash, subject, author, email string) gitlog.GraphLine {
		return gitlog.GraphLine{Graph: "*", Commit: &gitlog.GraphCommit{
			Hash: hash[:7], FullHash: hash, Subject: subject, Author: author, Email: email, Time: now,
		}}
	}
	lines := []gitlog.GraphLine{
		commit("aaaa111222", "Add parser", "Alice", "alice@x.io"),
		{Graph: "|"},
		commit("bbbb111222", "Fix parser crash", "Bob", "bob@y.io"),
		commit("cccc111222", "Fix docs", "Alice", "alice@y.io"),
	}
	m := New(stats.New([]gitlog.Commit{{Author: "Alice", Email: "alice@x.io", Time: now}}, nil), Options{View: GraphView})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	next, _ = next.(Model).Update(graphMsg{all: true, limit: graphPage, lines: lines})
	m = next.(Model)
	key := func(m Model, k tea.KeyType) Model {
		next, _ := m.Update(tea.KeyMsg{Type: k})
		return next.(Model)
	}
	search := func(m Model, query string) Model {
		m = press(m, "/", query)
		return key(m, tea.KeyEnter)
	}

	// Keys typed at the prompt are text, not commands.
	m = press(m, "/", "q")
	if !m.graph.typing || m.graph.input != "q" || !strings.Contains(m.View(), "/q") {
		t.Fatalf("prompt: typing %v, input %q", m.graph.typing, m.graph.input)
	}
	if m = key(m, tea.KeyEsc); m.graph.typing || m.graph.cursor != 0 {
		t.Errorf("esc should close the prompt in place: %+v", m.graph.cursor)
	}

	for _, tc := range []struct {
		query string
		want  int
	}{
		{"FIX", 2},
		{"fix @author alice", 3},
		{"@email y.io @author bob", 2},
		{"@commit CCCC", 3},
		{"@commit 1111", -1}, // a hash matches by prefix only
		{"parser @email x.io", 0},
	} {
		m.graph.cursor = 0
		m = search(m, tc.query)
		if got := m.graph.cursor; tc.want >= 0 && got != tc.want {
			t.Errorf("%q: cursor %d, want %d", tc.query, got, tc.want)
		}
		if tc.want < 0 && !strings.Contains(m.graph.status, "No later match") {
			t.Errorf("%q: status %q", tc.query, m.graph.status)
		}
	}

	// n and N step between matches and stop at the ends.
	m.graph.cursor = 0
	m = search(m, "fix")
	if m.graph.cursor != 2 || !strings.Contains(m.graph.status, "match 1 of 2") {
		t.Errorf("search: cursor %d, status %q", m.graph.cursor, m.graph.status)
	}
	if m = press(m, "n"); m.graph.cursor != 3 {
		t.Errorf("n: cursor %d", m.graph.cursor)
	}
	if m = press(m, "n"); m.graph.cursor != 3 || !strings.Contains(m.graph.status, "No later match") {
		t.Errorf("n past the last: cursor %d, status %q", m.graph.cursor, m.graph.status)
	}
	if m = press(m, "N", "N"); m.graph.cursor != 2 || !strings.Contains(m.graph.status, "No earlier match") {
		t.Errorf("N past the first: cursor %d, status %q", m.graph.cursor, m.graph.status)
	}

	// Matches are highlighted, and an empty search clears them.
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	mark := m.theme.match.Render("Fix")
	if !strings.Contains(m.graphLine(lines[3], false), mark) || strings.Contains(m.graphLine(lines[0], false), m.theme.match.Render("Add")) {
		t.Error("only matching commits should be highlighted")
	}
	if m = search(m, " "); m.graph.query != "" || strings.Contains(m.graphLine(lines[3], false), mark) {
		t.Errorf("empty search kept %q", m.graph.query)
	}

	// The prompt highlights as you type, and esc goes back to the last search.
	m = search(m, "add")
	if m = press(m, "/", "fix"); !strings.Contains(m.graphLine(lines[3], false), mark) {
		t.Error("typing does not highlight")
	}
	if m = key(m, tea.KeyEsc); strings.Contains(m.graphLine(lines[3], false), mark) ||
		!strings.Contains(m.graphLine(lines[0], false), m.theme.match.Render("Add")) {
		t.Error("esc should restore the last search's highlight")
	}

	// Tab completes a unique keyword prefix.
	m = key(press(m, "/", "fix @AU"), tea.KeyTab)
	if m.graph.input != "fix @author " {
		t.Errorf("tab completed to %q", m.graph.input)
	}
	if m = key(press(key(m, tea.KeyCtrlU), "@"), tea.KeyTab); m.graph.input != "@" {
		t.Errorf("ambiguous prefix completed to %q", m.graph.input)
	}
}

func TestHighlight(t *testing.T) {
	m := New(stats.New([]gitlog.Commit{{Author: "A", Email: "a@x", Time: time.Now()}}, nil), Options{})
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#000000"))
	match := lipgloss.NewStyle().Background(lipgloss.Color("#ffff00"))
	got := m.highlight("Fix fix FIX", "fix", false, base, match)
	if strings.Count(got, match.Render("Fix"))+strings.Count(got, match.Render("fix"))+strings.Count(got, match.Render("FIX")) != 3 {
		t.Errorf("every occurrence: %q", got)
	}
	if got := m.highlight("abcabc", "abc", true, base, match); strings.Count(got, match.Render("abc")) != 1 {
		t.Errorf("prefix: %q", got)
	}
	if got := m.highlight("xabc", "abc", true, base, match); strings.Contains(got, match.Render("abc")) {
		t.Errorf("prefix not at the start: %q", got)
	}
}

func TestGraphSearchLoadsHistory(t *testing.T) {
	now := time.Now()
	m := New(stats.New([]gitlog.Commit{{Author: "Alice", Email: "a@x", Time: now}}, nil), Options{View: GraphView})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	page := func(n int, subject string) []gitlog.GraphLine {
		var lines []gitlog.GraphLine
		for i := 0; i < n; i++ {
			lines = append(lines, gitlog.GraphLine{Graph: "*", Commit: &gitlog.GraphCommit{
				Hash: fmt.Sprintf("%07d", i), FullHash: fmt.Sprintf("%040d", i), Subject: "change", Time: now,
			}})
		}
		lines[n-1].Commit.Subject = subject
		return lines
	}
	next, _ = next.(Model).Update(graphMsg{all: true, limit: graphPage, lines: page(graphPage, "change")})
	m = press(next.(Model), "/", "release")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || m.graph.limit != 2*graphPage || m.graph.cursor != 0 {
		t.Fatalf("no older history requested: limit %d, cursor %d", m.graph.limit, m.graph.cursor)
	}
	next, _ = m.Update(graphMsg{all: true, limit: 2 * graphPage, lines: page(graphPage+10, "release")})
	if m = press(next.(Model), "n"); m.graph.cursor != graphPage+9 {
		t.Errorf("n after loading: cursor %d", m.graph.cursor)
	}
}
