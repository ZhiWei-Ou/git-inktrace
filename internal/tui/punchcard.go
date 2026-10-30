package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ZhiWei-Ou/git-inktrace/internal/stats"
)

// punchGlyphs grow with the intensity level; empty slots keep a faint dot
// so the grid stays visible.
var punchGlyphs = [5]string{"·", "•", "•", "●", "●"}

// Weekday hours outside workStart:00 to workEnd:00 count as after hours.
const workStart, workEnd = 9, 18

// Width of the weekday totals after the last hour.
const punchTotalWidth = 5

// punchCellWidth spaces the 24 hours and the weekday totals out when the
// terminal has room for it.
func (m Model) punchCellWidth() int {
	if m.width >= punchWidth(3)+punchTotalWidth {
		return 3
	}
	return 2
}

// punchWidth is the width of a weekday row without its total.
func punchWidth(cellWidth int) int { return gutter + 23*cellWidth + 1 }

// punchCard draws commits by weekday and hour of the author's clock, each
// slot in the color of its busiest author and sized by how busy it was,
// with weekday totals on the right and an hourly profile below.
func (m Model) punchCard() string {
	t := m.theme
	card := m.punch
	if card.Total == 0 {
		return " " + t.dim.Render("No commits in this range") + "\n"
	}
	cw := m.punchCellWidth()
	totals := m.width >= punchWidth(cw)+punchTotalWidth
	var b strings.Builder

	labels := []rune(strings.Repeat(" ", gutter+24*cw))
	for h := 0; h < 24; h += 3 {
		copy(labels[gutter+h*cw:], []rune(fmt.Sprintf("%02d", h)))
	}
	b.WriteString(t.dim.Render(strings.TrimRight(string(labels), " ")) + "\n")

	var hours [24]int
	for wd := 0; wd < 7; wd++ {
		b.WriteString(" " + t.dim.Render(fmt.Sprintf("%-4s", time.Weekday(wd).String()[:3])))
		total := 0
		for h, slot := range card.Slots[wd] {
			l := card.Level(slot.Total)
			if h > 0 {
				b.WriteString(strings.Repeat(" ", cw-1))
			}
			b.WriteString(lipgloss.NewStyle().Foreground(t.cellColor(slot.Top, l)).Render(punchGlyphs[l]))
			total += slot.Total
			hours[h] += slot.Total
		}
		if totals {
			b.WriteString(t.dim.Render(padLeft(fmt.Sprint(total), punchTotalWidth)))
		}
		b.WriteString("\n")
	}

	// Hourly profile: one bar per hour, scaled to the busiest hour.
	const bars = "▁▂▃▄▅▆▇█"
	peakHour := 0
	for _, n := range hours {
		peakHour = max(peakHour, n)
	}
	profile := strings.Repeat(" ", gutter)
	for _, n := range hours {
		bar := " "
		if n > 0 {
			bar = string([]rune(bars)[(n*len([]rune(bars))-1)/peakHour])
		}
		profile += bar + strings.Repeat(" ", cw-1)
	}
	b.WriteString(t.accent.Render(strings.TrimRight(profile, " ")) + "\n")

	scale := t.dim.Render(strings.Repeat(" ", gutter) + "Less ")
	for l := 0; l <= 4; l++ {
		scale += lipgloss.NewStyle().Foreground(t.cellColor(len(authorHues), l)).Render(punchGlyphs[l]) + " "
	}
	scale += t.dim.Render("More")
	summary := t.dim.Render(m.punchSummary())
	if line := spread(scale, summary+" ", m.width); line != scale {
		b.WriteString("\n" + line + "\n\n")
	} else {
		indent := strings.Repeat(" ", gutter)
		b.WriteString("\n" + scale + "\n" + indent + ansi.Truncate(summary, max(m.width-gutter-1, 0), "…") + "\n\n")
	}
	b.WriteString(m.punchLegend())
	return b.String()
}

// punchSummary names the busiest slot and the shares of commits made after
// weekday working hours and on weekends.
func (m Model) punchSummary() string {
	card := m.punch
	peakDay, peakHour, late, weekend := 0, 0, 0, 0
	for wd := range card.Slots {
		for h, slot := range card.Slots[wd] {
			if slot.Total > card.Slots[peakDay][peakHour].Total {
				peakDay, peakHour = wd, h
			}
			switch {
			case wd == int(time.Saturday) || wd == int(time.Sunday):
				weekend += slot.Total
			case h < workStart || h >= workEnd:
				late += slot.Total
			}
		}
	}
	return fmt.Sprintf("peak %s %02d:00 · %s after hours · %s weekends",
		time.Weekday(peakDay).String()[:3], peakHour, percent(late, card.Total), percent(weekend, card.Total))
}

// punchLegend names the authors whose colors appear on the card, folding
// the authors without their own hue into one entry.
func (m Model) punchLegend() string {
	t := m.theme
	seen := map[int]bool{}
	for _, row := range m.punch.Slots {
		for _, slot := range row {
			if slot.Top >= 0 {
				seen[min(slot.Top, len(authorHues))] = true
			}
		}
	}
	var parts []string
	for id := 0; id <= len(authorHues); id++ {
		if !seen[id] {
			continue
		}
		name := "others"
		if id < len(authorHues) {
			name = m.data.Authors[id].Name
		} else if m.author != stats.AllAuthors {
			name = m.data.Authors[m.author].Name // a filtered author without their own hue
		}
		parts = append(parts, lipgloss.NewStyle().Foreground(t.authorColor(id)).Render(cellGlyph)+" "+t.text.Render(name))
	}
	indent := strings.Repeat(" ", gutter)
	return wrap(indent, parts, indent, m.width-1) + "\n"
}
