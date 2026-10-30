package tui

import (
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/ZhiWei-Ou/git-inktrace/internal/gitlog"
	"github.com/ZhiWei-Ou/git-inktrace/internal/stats"
)

func testModel(t *testing.T, width int, authors ...string) Model {
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
	m := New(stats.New(commits, nil), Options{Repo: "demo", Branch: "main", Top: 100, Authors: authors})
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
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
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

func TestHeatmapCentered(t *testing.T) {
	m := testModel(t, 200)
	grid := gutter + maxWeeks*cellWidth
	pad := (200 - grid) / 2
	for _, line := range strings.Split(m.heatmap(), "\n") {
		if line == "" {
			continue
		}
		plain := ansi.Strip(line)
		if lead := len(plain) - len(strings.TrimLeft(plain, " ")); lead < pad {
			t.Errorf("line starts at %d, want at least %d: %q", lead, pad, plain)
		}
		if w := lipgloss.Width(line); w > pad+grid {
			t.Errorf("line ends at %d, past the block at %d: %q", w, pad+grid, plain)
		}
	}
	// A terminal the grid fills leaves nothing to center.
	if m := testModel(t, 80); !strings.Contains(ansi.Strip(m.heatmap()), "\n Mon ") {
		t.Errorf("80 columns shifted:\n%s", m.heatmap())
	}
}

func TestOverview(t *testing.T) {
	m := testModel(t, 120)
	created := time.Date(2024, 2, 29, 9, 0, 0, 0, time.Local)
	m.opts.Created = created
	out := ansi.Strip(m.heatmap())
	lines := strings.Split(out, "\n")
	// Each value is centered under its title, and the columns span the grid.
	grid := gutter + len(m.cal.Weeks)*cellWidth
	left := (120 - grid) / 2 // the heatmap block is centered too
	mid := func(line, s string) int { return strings.Index(line, s)*2 + len(s) }
	for i, title := range []string{"created", "age", "last commit", "commits", "authors"} {
		value := []string{"Feb 29, 2024", age(created, time.Now()), "today", "40", "3"}[i]
		if !strings.Contains(lines[0], title) || !strings.Contains(lines[1], value) {
			t.Fatalf("missing %s or %q:\n%s", title, value, out)
		}
		if d := mid(lines[0], title) - mid(lines[1], value); d < -1 || d > 1 {
			t.Errorf("%s and %q not centered on each other:\n%s", title, value, out)
		}
	}
	if strings.Index(lines[1], "Feb") < left+gutter || len(strings.TrimRight(lines[1], " ")) > left+grid {
		t.Errorf("overview outside the grid:\n%s", out)
	}
	// Columns that do not fit are left out.
	if m := testModel(t, 50); strings.Contains(ansi.Strip(m.heatmap()), "authors") {
		t.Errorf("50 columns kept every cell:\n%s", m.heatmap())
	}
}

func TestAge(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 12, 0, 0, 0, time.Local) }
	for _, tc := range []struct {
		from, to time.Time
		want     string
	}{
		{d(2026, 10, 30), d(2026, 10, 30), "0 days"},
		{d(2026, 10, 18), d(2026, 10, 30), "12 days"},
		{d(2025, 12, 21), d(2026, 10, 30), "10 months 9 days"},
		{d(2026, 1, 31), d(2026, 3, 1), "1 month 1 day"},
		{d(2024, 7, 26), d(2026, 10, 30), "2 years 3 months"},
		{d(2025, 10, 26), d(2026, 10, 30), "1 year"}, // no gap to days
	} {
		if got := age(tc.from, tc.to); got != tc.want {
			t.Errorf("age(%s, %s) = %q, want %q", formatDate(tc.from), formatDate(tc.to), got, tc.want)
		}
	}
	today := midnight(time.Now())
	for days, want := range map[int]string{0: "today", 1: "yesterday", 5: "5 days ago"} {
		if got := ago(today.AddDate(0, 0, -days).Add(13*time.Hour), today); got != want {
			t.Errorf("ago %d days = %q, want %q", days, got, want)
		}
	}
	if got := thousands(1234567); got != "1,234,567" {
		t.Errorf("thousands = %q", got)
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
	all := testModel(t, 120)
	m := testModel(t, 120, "alice", "CAROL@")
	if len(m.data.Authors) != 2 || m.cal.Total == 0 || m.cal.Total >= all.cal.Total {
		t.Errorf("filtered to %+v, total %d of %d", m.data.Authors, m.cal.Total, all.cal.Total)
	}
	out := ansi.Strip(m.View())
	for _, want := range []string{"Heatmap", "Graph", "■ Alice  ■ Carol"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Hours") || strings.Contains(out, "Owners") || strings.Contains(out, "Bob") {
		t.Errorf("hidden page or author shown:\n%s", out)
	}
	// Tabs and digits only reach the pages shown.
	if m = press(m, "tab", "tab", "tab"); m.view != HeatmapView {
		t.Errorf("tab wrapped to view %d", m.view)
	}
	if m = press(m, "4", "5"); m.view != HeatmapView {
		t.Errorf("hidden page %d opened", m.view)
	}
	if m = press(m, "3"); m.view != GraphView {
		t.Errorf("3 opened view %d", m.view)
	}
	help := ansi.Strip(press(m, "?").View())
	if !strings.Contains(help, "1 – 3") || strings.Contains(help, "Owners") || strings.Contains(help, "owners") {
		t.Errorf("help shows hidden pages:\n%s", help)
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

func TestHoursView(t *testing.T) {
	for _, width := range []int{60, 100} {
		m := press(testModel(t, width), "4")
		if m.view != HoursView || m.punch.Total != m.cal.Total || m.punch.Total == 0 {
			t.Fatalf("width %d: view %d, punch %d, calendar %d", width, m.view, m.punch.Total, m.cal.Total)
		}
		out := m.View()
		wants := []string{"Sun", "Sat", "21", "peak slot", "Alice", "hours 00–23"}
		if width >= 80 {
			// Summary and rhythm columns that need the room.
			wants = append(wants, "after hours", "weekends", "time zones", "UTC", "on each author's own clock")
		}
		for _, want := range wants {
			if !strings.Contains(out, want) {
				t.Errorf("width %d: punch card missing %q:\n%s", width, want, out)
			}
		}
		for _, line := range strings.Split(out, "\n") {
			if w := lipgloss.Width(line); w > width {
				t.Errorf("width %d: line %d wide: %q", width, w, line)
			}
		}
	}
	if m := press(testModel(t, 100), "4", "tab"); m.view != OwnersView {
		t.Errorf("tab from hours: view %d", m.view)
	}
}

func TestZones(t *testing.T) {
	for want, zs := range map[string]map[int]int{
		"UTC+0":              {0: 3},
		"UTC-5":              {-5 * 3600: 2},
		"UTC+5:30 · 2 zones": {19800: 4, 3600: 1},
	} {
		if got := zones(zs); got != want {
			t.Errorf("zones(%v) = %q, want %q", zs, got, want)
		}
	}
}

func TestHoursRhythm(t *testing.T) {
	tokyo, ny := time.FixedZone("JST", 9*3600), time.FixedZone("EST", -5*3600)
	// Mon 2026-10-26; Alice works mornings in Tokyo, Bob late evenings and
	// Saturdays in New York.
	var commits []gitlog.Commit
	for d := 0; d < 4; d++ {
		commits = append(commits,
			gitlog.Commit{Author: "Alice", Email: "a@x", Time: time.Date(2026, 10, 26+d, 10, 0, 0, 0, tokyo)},
			gitlog.Commit{Author: "Alice", Email: "a@x", Time: time.Date(2026, 10, 26+d, 11, 0, 0, 0, tokyo)},
			gitlog.Commit{Author: "Bob", Email: "b@x", Time: time.Date(2026, 10, 24, 20+d, 0, 0, 0, ny)})
	}
	m := New(stats.New(commits, nil), Options{View: HoursView})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(Model)
	out := ansi.Strip(m.View())
	for _, want := range []string{"Mon 10:00", "10:00", "33.3%", "UTC+9 · 2 zones"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
	row := func(name string) string {
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "■ "+name) {
				return l
			}
		}
		t.Fatalf("no row for %s:\n%s", name, out)
		return ""
	}
	// Alice: peak 10:00, none after hours or on weekends; Bob: 20:00, all
	// on a Saturday.
	if a := strings.Fields(row("Alice")); a[len(a)-3] != "10:00" || a[len(a)-2] != "0.0%" || a[len(a)-1] != "0.0%" {
		t.Errorf("Alice row: %q", a)
	}
	if b := strings.Fields(row("Bob")); b[len(b)-3] != "20:00" || b[len(b)-1] != "100.0%" {
		t.Errorf("Bob row: %q", b)
	}
	// The block is centered.
	if lead := len(row("Alice")) - len(strings.TrimLeft(row("Alice"), " ")); lead < (120-80)/2 {
		t.Errorf("not centered: %q", row("Alice"))
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

	// o copies the selected commit's full hash.
	var copied string
	copyToClipboard = func(s string) error { copied = s; return nil }
	defer func() { copyToClipboard = clipboard.WriteAll }()
	o := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")}
	_, cmd = m.Update(o)
	if cmd == nil {
		t.Fatal("o produced no command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if copied != "full-0000019" || !strings.Contains(m.View(), "Copied full-0000019") {
		t.Errorf("copied %q", copied)
	}
	copyToClipboard = func(string) error { return errors.New("no clipboard utility") }
	_, cmd = m.Update(o)
	next, _ = m.Update(cmd())
	if !strings.Contains(next.(Model).View(), "Copy failed: no clipboard utility") {
		t.Error("copy failure not reported")
	}

	// Switching scope reloads from scratch, and stale results are dropped.
	m = press(m, "a")
	if m.graph.all || !m.graph.loading || len(m.graph.lines) != 0 {
		t.Errorf("a did not switch to the current branch: %+v", m.graph)
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
	if m := New(data, Options{View: HoursView}); m.view != HoursView || m.Init() != nil {
		t.Errorf("hours start: view %d", m.view)
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

	// The prompt highlights as you type, and esc leaves it without any
	// highlight, also dropping the last search.
	m = search(m, "add")
	if m = press(m, "/", "fix"); !strings.Contains(m.graphLine(lines[3], false), mark) {
		t.Error("typing does not highlight")
	}
	if m = key(m, tea.KeyEsc); strings.Contains(m.graphLine(lines[3], false), mark) ||
		strings.Contains(m.graphLine(lines[0], false), m.theme.match.Render("Add")) || m.graph.query != "" {
		t.Error("esc should clear the highlight")
	}
	// Outside the prompt esc clears the last search too, and n finds nothing.
	m = search(m, "add")
	if m = key(m, tea.KeyEsc); strings.Contains(m.graphLine(lines[0], false), m.theme.match.Render("Add")) ||
		strings.Contains(m.View(), "/add") {
		t.Error("esc after a search should clear the highlight")
	}
	if cursor := m.graph.cursor; press(m, "n").graph.cursor != cursor {
		t.Error("n moved after the search was cleared")
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

func TestQuitKeys(t *testing.T) {
	m := New(stats.New([]gitlog.Commit{{Author: "A", Email: "a@x", Time: time.Now()}}, nil), Options{})
	quits := func(msg tea.KeyMsg) bool {
		_, cmd := m.Update(msg)
		if cmd == nil {
			return false
		}
		_, ok := cmd().(tea.QuitMsg)
		return ok
	}
	if !quits(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}) || !quits(tea.KeyMsg{Type: tea.KeyCtrlC}) {
		t.Error("q and ctrl+c should quit")
	}
	if quits(tea.KeyMsg{Type: tea.KeyEsc}) {
		t.Error("esc should not quit")
	}
}

func TestInspectFile(t *testing.T) {
	now := time.Now()
	commits := []gitlog.Commit{
		{Hash: "c3c3c3c3c3", Subject: "tune parser", Author: "Bob", Email: "bob@x", Time: now,
			Files: []gitlog.FileChange{{Path: "parser.go", Added: 4, Deleted: 2}}},
		{Hash: "b2b2b2b2b2", Subject: "add readme", Author: "Alice", Email: "alice@x", Time: now.AddDate(0, 0, -1),
			Files: []gitlog.FileChange{{Path: "README.md", Added: 1}}},
		{Hash: "a1a1a1a1a1", Subject: "add parser", Author: "Alice", Email: "alice@x", Time: now.AddDate(0, 0, -2),
			Files: []gitlog.FileChange{{Path: "parser.go", Added: 30}, {Path: "README.md", Added: 3}}},
	}
	m := New(stats.New(commits, nil), Options{View: FilesView})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m = next.(Model)

	// The cursor selects a ranked file; enter lists the commits touching it.
	if m = press(m, "j"); m.cursor != 1 || m.files[1].Path != "README.md" {
		t.Fatalf("cursor %d on %+v", m.cursor, m.files)
	}
	m = press(m, "k", "enter")
	out := m.View()
	if !m.inspect.open || len(m.inspect.commits) != 2 || !strings.Contains(out, "tune parser") ||
		!strings.Contains(out, "c3c3c3c") || strings.Contains(out, "add readme") {
		t.Fatalf("inspect parser.go:\n%s", out)
	}
	// Navigation keys scroll the page, which has no selection, instead of
	// moving through the ranking.
	if m = press(m, "j", "G"); m.inspect.scroll != 0 || m.cursor != 0 {
		t.Errorf("scrolled %d with all rows visible, ranking cursor %d", m.inspect.scroll, m.cursor)
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 8}) // one row of the list
	if m = press(next.(Model), "j", "j"); m.inspect.scroll != 1 || !strings.Contains(m.View(), "add parser") {
		t.Errorf("scroll %d:\n%s", m.inspect.scroll, m.View())
	}
	if m = press(m, "g"); m.inspect.scroll != 0 {
		t.Errorf("g: scroll %d", m.inspect.scroll)
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Error("enter should do nothing on the page")
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m = next.(Model); m.inspect.open || m.view != FilesView {
		t.Errorf("esc should close the page: open %v, view %d", m.inspect.open, m.view)
	}
	m = press(m, "enter")
	if next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd != nil || next.(Model).inspect.open {
		t.Errorf("q should close the page, not quit: open %v", next.(Model).inspect.open)
	}
	if m = press(m, "enter", "tab"); m.inspect.open || m.view != GraphView {
		t.Errorf("tab should close the page and switch: open %v, view %d", m.inspect.open, m.view)
	}
}

func TestInspectCommit(t *testing.T) {
	now := time.Now()
	m := New(stats.New([]gitlog.Commit{{Author: "Alice", Email: "a@x", Time: now}}, nil), Options{View: GraphView})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	commit := func(hash string) gitlog.GraphLine {
		return gitlog.GraphLine{Graph: "*", Commit: &gitlog.GraphCommit{
			Hash: hash[:7], FullHash: hash, Subject: "commit " + hash, Author: "Alice", Email: "a@x", Time: now,
		}}
	}
	next, _ = next.(Model).Update(graphMsg{all: true, limit: graphPage, lines: []gitlog.GraphLine{commit("aaaaaaaaaa"), commit("bbbbbbbbbb")}})
	m = press(next.(Model), "j")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || !m.inspect.open || m.inspect.commit.FullHash != "bbbbbbbbbb" ||
		!strings.Contains(m.View(), "Loading changed files") {
		t.Fatalf("inspect not loading: %+v", m.inspect)
	}

	// A result for another commit is ignored.
	next, _ = m.Update(inspectMsg{hash: "aaaaaaaaaa", files: []gitlog.FileChange{{Path: "wrong.go"}}})
	files := []gitlog.FileChange{
		{Path: "main.go", Added: 10, Deleted: 5},
		{Path: "new/name.go", OldPath: "old/name.go"},
		{Path: "logo.png", Added: -1, Deleted: -1},
	}
	body := "Explain why.\n\n" + strings.Repeat("word ", 40) + "end\n\nSigned-off-by: Alice <a@x>"
	next, _ = next.(Model).Update(inspectMsg{hash: "bbbbbbbbbb", body: body, files: files})
	out := next.(Model).View()
	// The whole body shows, wrapped to the screen, before the files.
	if !strings.Contains(out, "Signed-off-by: Alice <a@x>") || !strings.Contains(out, "word end") ||
		strings.Index(out, "Explain why.") > strings.Index(out, "main.go") {
		t.Errorf("body missing or misplaced:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > 120 {
			t.Errorf("line %d wide: %q", lipgloss.Width(line), line)
		}
	}
	for _, want := range []string{"commit bbbbbbbbbb", "main.go", "+10", "-5", "++++", "old/name.go → new/name.go", "bin", "3 files changed, +10 -5"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "wrong.go") {
		t.Error("stale result shown")
	}
}

func TestGraphReword(t *testing.T) {
	now := time.Now()
	commit := func(hash string) gitlog.GraphLine {
		return gitlog.GraphLine{Graph: "*", Commit: &gitlog.GraphCommit{
			Hash: hash[:7], FullHash: hash, Subject: "commit " + hash, Author: "Alice", Email: "a@x", Time: now,
		}}
	}
	m := New(stats.New([]gitlog.Commit{{Author: "Alice", Email: "a@x", Time: now}}, nil), Options{View: GraphView})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	next, _ = next.(Model).Update(graphMsg{all: true, limit: graphPage, lines: []gitlog.GraphLine{commit("aaaaaaaaaa"), commit("bbbbbbbbbb")}})
	m = press(next.(Model), "j")

	var remotes []string
	var checkErr error
	checkReword = func(dir, hash string) ([]string, error) {
		if hash != "bbbbbbbbbb" {
			t.Errorf("checked %s", hash)
		}
		return remotes, checkErr
	}
	defer func() { checkReword = gitlog.CheckReword }()
	check := func(m Model) Model {
		t.Helper()
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
		if cmd == nil {
			t.Fatal("e started nothing")
		}
		next, _ = next.(Model).Update(cmd())
		return next.(Model)
	}

	checkErr = errors.New("the commit is not on the current branch")
	if m = check(m); !strings.Contains(m.View(), "Cannot reword bbbbbbb: the commit is not on the current branch") {
		t.Errorf("check error not shown:\n%s", m.View())
	}

	// A pushed commit asks first; anything but y cancels.
	checkErr, remotes = nil, []string{"origin/main", "origin/dev", "fork/main"}
	m = check(m)
	if out := m.View(); m.graph.confirm == nil || !strings.Contains(out, "bbbbbbb is already on origin/main, origin/dev and 1 more") ||
		!strings.Contains(out, "[y/N]") {
		t.Fatalf("no confirmation:\n%s", out)
	}
	if m = press(m, "j"); m.graph.confirm != nil || m.graph.cursor != 1 || !strings.Contains(m.View(), "Reword cancelled") {
		t.Errorf("j should cancel, not move: cursor %d\n%s", m.graph.cursor, m.View())
	}
	// y opens the editor.
	editMessageCommand = func(dir, hash string) (*exec.Cmd, string, error) {
		return nil, "", errors.New("no editor")
	}
	defer func() { editMessageCommand = gitlog.EditMessageCommand }()
	if m = press(check(m), "y"); m.graph.confirm != nil || !strings.Contains(m.View(), "Cannot edit bbbbbbb: no editor") {
		t.Errorf("y did not reach the editor:\n%s", m.View())
	}

	next, _ = m.Update(rewordedMsg{commit: m.graph.lines[1].Commit, err: gitlog.ErrMessageUnchanged})
	if m = next.(Model); !strings.Contains(m.View(), "Message unchanged, bbbbbbb kept") {
		t.Errorf("unchanged not shown:\n%s", m.View())
	}

	// After a reword the graph reloads, ignoring pages read before it, and
	// selects the new commit.
	next, cmd := m.Update(rewordedMsg{commit: m.graph.lines[1].Commit, hash: "cccccccccc"})
	m = next.(Model)
	if cmd == nil || !m.graph.loading || !strings.Contains(m.View(), "Reworded bbbbbbb → ccccccc") {
		t.Fatalf("no reload after reword: %+v", m.graph)
	}
	next, _ = m.Update(graphMsg{seq: m.graph.seq - 1, all: true, limit: graphPage, lines: []gitlog.GraphLine{commit("aaaaaaaaaa")}})
	if m = next.(Model); len(m.graph.lines) != 2 {
		t.Error("stale page accepted")
	}
	next, _ = m.Update(graphMsg{seq: m.graph.seq, all: true, limit: graphPage,
		lines: []gitlog.GraphLine{commit("dddddddddd"), {Graph: "|"}, commit("cccccccccc")}})
	if m = next.(Model); m.graph.cursor != 2 || m.graph.focus != "" {
		t.Errorf("cursor %d, focus %q", m.graph.cursor, m.graph.focus)
	}
}

func TestHelpPage(t *testing.T) {
	m := testModel(t, 80)
	if out := m.View(); !strings.Contains(out, "? help") || strings.Contains(out, "reword message") {
		t.Fatalf("footer:\n%s", out)
	}
	m = press(m, "?")
	out := m.View()
	for _, want := range []string{"General", "Time window", "Graph", "reword message", "Inspect", "? close"} {
		if !strings.Contains(out, want) {
			t.Errorf("help page missing %q:\n%s", want, out)
		}
	}
	for _, width := range []int{80, 50} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		for _, line := range strings.Split(next.(Model).View(), "\n") {
			if w := lipgloss.Width(line); w > width {
				t.Errorf("width %d: line %d wide: %q", width, w, line)
			}
		}
	}
	// Two columns fit in 80, so the page fits in 24 lines.
	if next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}); strings.Count(next.(Model).View(), "\n") > 23 {
		t.Errorf("help page taller than 24 lines:\n%s", next.(Model).View())
	}

	// Page keys do nothing behind the help; ?, q and esc close it.
	if m = press(m, "tab", "2"); !m.helpOpen || m.view != HeatmapView {
		t.Errorf("keys reached the page: open %v, view %d", m.helpOpen, m.view)
	}
	if next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd != nil || next.(Model).helpOpen {
		t.Error("q should close the help, not quit")
	}
	if next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc}); next.(Model).helpOpen {
		t.Error("esc should close the help")
	}
	if m = press(m, "?"); m.helpOpen {
		t.Error("? should close the help")
	}
	if _, cmd := press(m, "?").Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil {
		t.Error("ctrl+c should quit from the help")
	}

	// It opens over an inspect page too, and ? is plain text at the search prompt.
	m = press(testModel(t, 80), "tab", "enter", "?")
	if !m.helpOpen || !m.inspect.open {
		t.Errorf("help over inspect: open %v, inspect %v", m.helpOpen, m.inspect.open)
	}
	if m = press(m, "q"); m.helpOpen || !m.inspect.open {
		t.Error("closing the help should return to the inspect page")
	}
	m = press(testModel(t, 80), "3", "/", "?")
	if m.helpOpen || m.graph.input != "?" {
		t.Errorf("? at the prompt: open %v, input %q", m.helpOpen, m.graph.input)
	}
}

// runIndex runs the indexing command of a batch, skipping the progress tick.
func runIndex(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for _, c := range cmd().(tea.BatchMsg) {
		if c == nil {
			continue
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- c() }()
		select {
		case msg := <-done:
			if msg, ok := msg.(indexedMsg); ok {
				next, _ := m.Update(msg)
				return next.(Model)
			}
		case <-time.After(50 * time.Millisecond): // the tick
		}
	}
	t.Fatal("no indexing in the batch")
	return m
}

func TestBackgroundIndex(t *testing.T) {
	now := time.Now()
	header := func(hash string, d int) gitlog.Commit {
		return gitlog.Commit{Hash: hash, Author: "Alice", Email: "a@x", Time: now.AddDate(0, 0, -d)}
	}
	commits := []gitlog.Commit{header("bbb", 0), header("aaa", 1)}
	var asked []string
	readFiles = func(dir string, cs []gitlog.Commit, workers int, done *atomic.Int64) ([][]gitlog.FileChange, error) {
		files := make([][]gitlog.FileChange, len(cs))
		for i, c := range cs {
			asked = append(asked, c.Hash)
			files[i] = []gitlog.FileChange{{Path: c.Hash + ".go", Added: 1}}
			done.Add(1)
		}
		return files, nil
	}
	defer func() { readFiles = gitlog.ReadFiles }()

	m := New(stats.New(commits, nil), Options{View: FilesView, Commits: commits})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = next.(Model)
	if out := m.View(); !m.index.loading || !strings.Contains(out, "Reading changed files") || !strings.Contains(out, "0 / 2 commits") {
		t.Fatalf("no progress while indexing:\n%s", out)
	}
	m = runIndex(t, m, m.Init())
	if out := m.View(); m.index.loading || len(m.files) != 2 || !strings.Contains(out, "aaa.go") || strings.Contains(out, "Reading") {
		t.Fatalf("ranking after indexing:\n%s", out)
	}

	// After history changes, known commits keep their files at once and
	// only the new one is read.
	asked = nil
	next, cmd := m.Update(dataMsg{commits: []gitlog.Commit{header("ccc", 0), header("aaa", 1)}})
	m = next.(Model)
	if len(m.files) != 1 || m.files[0].Path != "aaa.go" || m.index.total != 1 || !strings.Contains(m.View(), "Reading changed files") {
		t.Fatalf("reloaded ranking %+v, total %d", m.files, m.index.total)
	}
	stale := indexedMsg{gen: m.index.gen - 1, data: stats.New(nil, nil)}
	if next, _ := m.Update(stale); len(next.(Model).files) != 1 {
		t.Error("stale indexing accepted")
	}
	m = runIndex(t, m, cmd)
	if strings.Join(asked, " ") != "ccc" || len(m.files) != 2 || m.files[1].Path != "ccc.go" && m.files[0].Path != "ccc.go" {
		t.Errorf("asked %v, files %+v", asked, m.files)
	}
	if _, ok := m.index.cache["bbb"]; ok {
		t.Error("cache keeps a commit no longer in history")
	}

	// A failure is shown in place of the ranking.
	readFiles = func(string, []gitlog.Commit, int, *atomic.Int64) ([][]gitlog.FileChange, error) {
		return nil, errors.New("boom")
	}
	m = New(stats.New(commits, nil), Options{View: FilesView, Commits: commits})
	if m = runIndex(t, m, m.Init()); !strings.Contains(m.View(), "Cannot read changed files: boom") {
		t.Errorf("error not shown:\n%s", m.View())
	}
}

func TestGraphStartsOnCurrentBranchWithManyBranches(t *testing.T) {
	data := stats.New([]gitlog.Commit{{Author: "A", Email: "a@x", Time: time.Now()}}, nil)
	if m := New(data, Options{Branches: allBranchesLimit}); !m.graph.all {
		t.Error("few branches should graph all of them")
	}
	if m := New(data, Options{Branches: allBranchesLimit + 1}); m.graph.all || !strings.Contains(m.graphScope(), "current branch") {
		t.Error("many branches should graph the current one")
	}
}

func ownersModel(t *testing.T, width, height int) Model {
	t.Helper()
	now := time.Now()
	change := func(name, path string, added int) gitlog.Commit {
		return gitlog.Commit{Author: name, Email: strings.ToLower(name) + "@x", Time: now,
			Files: []gitlog.FileChange{{Path: path, Added: added, Deleted: 1}}}
	}
	var commits []gitlog.Commit
	for i := 0; i < 12; i++ {
		commits = append(commits, change("Alice", "src/core/engine.go", 10))
	}
	for i := 0; i < 6; i++ {
		commits = append(commits, change([]string{"Bob", "Carol"}[i%2], "src/ui/view.go", 100))
	}
	for i := 0; i < 4; i++ {
		commits = append(commits, change("Bob", "docs/guide/intro.md", 1))
	}
	commits = append(commits, change("Alice", "README.md", 1), change("Carol", "old.go", 1))
	tracked := []string{"src/core/engine.go", "src/ui/view.go", "docs/guide/intro.md", "README.md"}
	m := New(stats.New(commits, tracked), Options{Repo: "demo", View: OwnersView})
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(Model)
}

func TestBuildOwners(t *testing.T) {
	m := ownersModel(t, 100, 30)
	root := m.owners.root
	// Deleted files are left out; docs/guide is one tile.
	var names []string
	for _, c := range root.children {
		names = append(names, c.name)
	}
	if strings.Join(names, " ") != "src docs/guide README.md" {
		t.Fatalf("root children: %v", names)
	}
	src := root.find("src")
	if src == nil || src.changes != 18 || src.added != 12*10+6*100 || len(src.byAuthor) != 3 {
		t.Fatalf("src = %+v", src)
	}
	if top, share, bus := src.owner(); top != m.data.AuthorOf("Alice", "alice@x") || share < 0.66 || share > 0.67 || bus != 1 {
		t.Errorf("src owner %d %.2f bus %d", top, share, bus)
	}
	// Bob and Carol split src/ui evenly: neither alone made over half.
	if _, _, bus := root.find("src/ui").owner(); bus != 2 {
		t.Errorf("ui bus factor %d", bus)
	}
	if root.parent(root.find("src/ui")) != src || root.find("docs/guide").dir != true {
		t.Error("parent or find wrong")
	}
}

func TestSquarify(t *testing.T) {
	r := rect{0, 0, 40, 20}
	rects := squarify([]float64{50, 25, 12, 8, 5}, r)
	area := 0.0
	for i, a := range rects {
		area += a.w * a.h
		if a.x < -1e-9 || a.y < -1e-9 || a.x+a.w > r.w+1e-9 || a.y+a.h > r.h+1e-9 {
			t.Errorf("rect %d outside: %+v", i, a)
		}
		for j, b := range rects[:i] {
			if a.x+1e-9 < b.x+b.w && b.x+1e-9 < a.x+a.w && a.y+1e-9 < b.y+b.h && b.y+1e-9 < a.y+a.h {
				t.Errorf("rects %d and %d overlap: %+v %+v", j, i, b, a)
			}
		}
	}
	if math.Abs(area-r.w*r.h) > 1e-6 || math.Abs(rects[0].w*rects[0].h-400) > 1e-6 {
		t.Errorf("areas wrong: total %.2f, first %+v", area, rects[0])
	}
}

func TestOwnersView(t *testing.T) {
	m := ownersModel(t, 100, 30)
	out := ansi.Strip(m.View())
	for _, want := range []string{"Owners", "demo/", "area: file changes", " src/", "docs/guide/", "Alice 67%", "18 changes", "bus factor 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "old.go") {
		t.Error("deleted file shown")
	}
	// The selected tile keeps its full color; the others fade.
	if tl := *m.selectedTile(); m.tileStyle(tl, true).GetBackground() == m.tileStyle(tl, false).GetBackground() {
		t.Errorf("selected tile not set apart: %v", m.tileStyle(tl, true).GetBackground())
	}
	lines := strings.Split(m.View(), "\n")
	if len(lines) > 30 {
		t.Errorf("%d lines for 30 rows", len(lines))
	}
	for _, l := range lines {
		if w := lipgloss.Width(l); w > 100 {
			t.Errorf("line %d wide", w)
		}
	}

	// Enter opens a directory, esc goes back and selects it again.
	m = press(m, "enter")
	if m.owners.dir != "src" || !strings.Contains(ansi.Strip(m.View()), "demo/src/") {
		t.Fatalf("enter: dir %q", m.owners.dir)
	}
	m = press(m, "j")
	sel := m.selectedTile()
	if sel == nil || sel.node.path != "src/ui" {
		t.Fatalf("second tile in src: %+v", sel)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m = next.(Model); m.owners.dir != "" || m.selectedTile().node.path != "src" {
		t.Errorf("esc: dir %q, selected %s", m.owners.dir, m.selectedTile().node.path)
	}

	// w sizes by lines, where src/ui outweighs src/core.
	m = press(m, "enter", "w")
	if !strings.Contains(m.View(), "area: lines changed") || m.selectedTile().node.path != "src/ui" {
		t.Errorf("w: selected %s", m.selectedTile().node.path)
	}

	// Enter on a file opens its commits.
	m = press(m, "enter", "enter")
	if !m.inspect.open || m.inspect.path != "src/ui/view.go" || len(m.inspect.commits) != 6 {
		t.Errorf("inspect %+v", m.inspect)
	}
}

func TestOwnersFoldsSmallTiles(t *testing.T) {
	now := time.Now()
	var commits []gitlog.Commit
	var tracked []string
	for i := 0; i < 40; i++ {
		p := fmt.Sprintf("pkg%02d.go", i)
		tracked = append(tracked, p)
		for j := 0; j <= 40-i; j++ {
			commits = append(commits, gitlog.Commit{Author: "A", Email: "a@x", Time: now,
				Files: []gitlog.FileChange{{Path: p, Added: 1}}})
		}
	}
	m := New(stats.New(commits, tracked), Options{View: OwnersView})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(Model)
	w, h := m.ownersMapSize()
	tiles := m.ownersTiles(m.owners.root, w, h)
	folded := 0
	for _, tl := range tiles {
		folded += tl.folded
	}
	if folded == 0 || len(tiles)+folded-1 != 40 || !strings.Contains(m.View(), fmt.Sprintf("+%d more", folded)) {
		t.Errorf("no folded tile: %d tiles, %d folded", len(tiles), folded)
	}
	for _, tl := range tiles {
		if tl.node != nil && (tl.x1-tl.x0 < minTileWidth || tl.y1-tl.y0 < minTileHeight) {
			t.Errorf("tile too small: %+v", tl)
		}
	}
}
