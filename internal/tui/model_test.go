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
	m := New(stats.New(commits), Options{Repo: "demo", Branch: "main", Top: 100})
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
	m := New(stats.New(commits), Options{Top: 10})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(Model)
	end := m.cal.End

	m = press(m, "right")
	if !m.cal.End.Equal(end) {
		t.Errorf("moved past the latest window: %v", m.cal.End)
	}
	m = press(m, "left", "h")
	if want := end.AddDate(-2, 0, 0); !m.cal.End.Equal(want) {
		t.Errorf("after two years back: end %v, want %v", m.cal.End, want)
	}
	m = press(m, "left")
	if want := end.AddDate(-2, 0, 0); !m.cal.End.Equal(want) {
		t.Errorf("moved before the first commit: end %v", m.cal.End)
	}
	m = press(m, "l")
	if want := end.AddDate(-1, 0, 0); !m.cal.End.Equal(want) || m.cal.Total == 0 {
		t.Errorf("forward: end %v, total %d", m.cal.End, m.cal.Total)
	}
	m = press(m, "t")
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
	m := New(stats.New(commits), Options{Top: 10})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	out := next.(Model).View()

	for _, want := range []string{"this window", "all history", "Alice <alice@example.com>", "100.0%", "33.3%", "66.7%"} {
		if !strings.Contains(out, want) {
			t.Errorf("legend missing %q:\n%s", want, out)
		}
	}
}
