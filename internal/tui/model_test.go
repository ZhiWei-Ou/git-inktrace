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

func TestNavigation(t *testing.T) {
	m := testModel(t, 120)
	end := m.cal.End

	m = press(m, "left")
	if !m.cursor.Equal(end.AddDate(0, 0, -7)) {
		t.Errorf("cursor = %v, want a week before %v", m.cursor, end)
	}

	m = press(m, "[")
	if !m.cal.End.Before(end) || m.cursor.Before(m.cal.Start) || m.cursor.After(m.cal.End) {
		t.Errorf("after [: window %v..%v, cursor %v", m.cal.Start, m.cal.End, m.cursor)
	}
	m = press(m, "]", "]")
	if !m.cal.End.Equal(end) {
		t.Errorf("] went past today: %v", m.cal.End)
	}

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
