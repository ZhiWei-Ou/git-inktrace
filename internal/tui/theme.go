package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// authorHues are distinct base colors, one per top-ranked author.
var authorHues = []string{
	"#2da44e", // green
	"#3b82f6", // blue
	"#f97316", // orange
	"#a855f7", // purple
	"#ec4899", // pink
	"#06b6d4", // cyan
	"#eab308", // yellow
	"#ef4444", // red
}

const othersHue = "#8b949e"

// levelMix is how far each intensity level blends from the empty cell color
// toward the author hue.
var levelMix = [5]float64{0, 0.4, 0.6, 0.8, 1}

type theme struct {
	dark bool
	// cells[i][level] is the cell color of hue i at an intensity level;
	// the last entry is the shared "others" hue.
	cells [][5]lipgloss.Color

	title, dim, text, accent, tab, activeTab, match lipgloss.Style
}

func newTheme(dark bool) theme {
	base, empty := "#ffffff", "#ebedf0"
	fg, dim, accent, mark := "#24292f", "#6e7781", "#0969da", "#fae17d"
	if dark {
		base, empty = "#0d1117", "#2d333b"
		fg, dim, accent, mark = "#e6edf3", "#7d8590", "#58a6ff", "#d29922"
	}
	t := theme{dark: dark}
	for _, hue := range append(authorHues[:len(authorHues):len(authorHues)], othersHue) {
		var shades [5]lipgloss.Color
		shades[0] = lipgloss.Color(empty)
		for l := 1; l < 5; l++ {
			shades[l] = lipgloss.Color(mix(empty, hue, levelMix[l]))
		}
		t.cells = append(t.cells, shades)
	}
	t.text = lipgloss.NewStyle().Foreground(lipgloss.Color(fg))
	t.dim = lipgloss.NewStyle().Foreground(lipgloss.Color(dim))
	t.accent = lipgloss.NewStyle().Foreground(lipgloss.Color(accent))
	t.title = t.accent.Bold(true)
	t.tab = t.dim.Padding(0, 1)
	t.activeTab = lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Foreground(lipgloss.Color(base)).Background(lipgloss.Color(accent))
	t.match = lipgloss.NewStyle().Foreground(lipgloss.Color("#24292f")).Background(lipgloss.Color(mark))
	return t
}

// cellColor returns the color for an author index (-1 for none) at a level.
func (t theme) cellColor(author, level int) lipgloss.Color {
	if author < 0 || level == 0 {
		return t.cells[0][0]
	}
	return t.cells[min(author, len(t.cells)-1)][level]
}

// authorColor is the full-intensity color of an author.
func (t theme) authorColor(author int) lipgloss.Color {
	return t.cellColor(author, 4)
}

// mix blends two #rrggbb colors; f=0 yields a, f=1 yields b.
func mix(a, b string, f float64) string {
	var ar, ag, ab, br, bg, bb int
	fmt.Sscanf(a, "#%02x%02x%02x", &ar, &ag, &ab)
	fmt.Sscanf(b, "#%02x%02x%02x", &br, &bg, &bb)
	lerp := func(x, y int) int { return int(float64(x) + (float64(y)-float64(x))*f + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", lerp(ar, br), lerp(ag, bg), lerp(ab, bb))
}
