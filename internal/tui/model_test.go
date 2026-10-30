package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
	if m.view != filesView || len(m.files) != 5 {
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
