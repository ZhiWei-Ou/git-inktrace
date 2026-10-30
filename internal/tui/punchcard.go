package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// punchGlyphs grow with the intensity level; empty slots keep a faint dot
// so the grid stays visible.
var punchGlyphs = [5]string{"·", "•", "•", "●", "●"}

// Weekday hours outside workStart:00 to workEnd:00 count as after hours.
const workStart, workEnd = 9, 18

// Width of the weekday totals after the last hour.
const punchTotalWidth = 5

// sparkBars draw an hourly profile, scaled to its busiest hour.
const sparkBars = "▁▂▃▄▅▆▇█"

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

// rhythm sums up when the commits in a weekday-hour grid were made.
type rhythm struct {
	total             int
	peakDay, peakHour int
	hours             [24]int
	late, weekend     int // after weekday working hours, and on weekends
}

func newRhythm(grid *[7][24]int) rhythm {
	var r rhythm
	for wd := range grid {
		for h, n := range grid[wd] {
			r.total += n
			r.hours[h] += n
			if n > grid[r.peakDay][r.peakHour] {
				r.peakDay, r.peakHour = wd, h
			}
			switch {
			case wd == int(time.Saturday) || wd == int(time.Sunday):
				r.weekend += n
			case h < workStart || h >= workEnd:
				r.late += n
			}
		}
	}
	return r
}

// busiestHour is the hour of the day with the most commits over all weekdays.
func (r rhythm) busiestHour() int {
	best := 0
	for h, n := range r.hours {
		if n > r.hours[best] {
			best = h
		}
	}
	return best
}

// spark draws the hourly profile, one bar per hour spaced cw columns apart.
func (r rhythm) spark(cw int) string {
	most := 0
	for _, n := range r.hours {
		most = max(most, n)
	}
	bars := []rune(sparkBars)
	var b strings.Builder
	for h, n := range r.hours {
		if h > 0 {
			b.WriteString(strings.Repeat(" ", cw-1))
		}
		if n == 0 {
			b.WriteString(" ")
			continue
		}
		b.WriteRune(bars[(n*len(bars)-1)/most])
	}
	return b.String()
}

// punchCard draws commits by weekday and hour of the author's clock, each
// slot in the color of its busiest author and sized by how busy it was.
// A summary sits above, weekday totals on the right, an hourly profile
// below and each author's own rhythm under that, centered as one block.
func (m Model) punchCard() string {
	t := m.theme
	card := m.punch
	if card.Total == 0 {
		return " " + t.dim.Render("No commits in this range") + "\n"
	}
	cw := m.punchCellWidth()
	width := punchWidth(cw)
	totals := m.width >= width+punchTotalWidth
	if totals {
		width += punchTotalWidth
	}
	var grid [7][24]int
	for wd := range card.Slots {
		for h, slot := range card.Slots[wd] {
			grid[wd][h] = slot.Total
		}
	}
	all := newRhythm(&grid)

	var b strings.Builder
	b.WriteString(m.statRow([][2]string{
		{"peak slot", fmt.Sprintf("%s %02d:00", time.Weekday(all.peakDay).String()[:3], all.peakHour)},
		{"busiest hour", fmt.Sprintf("%02d:00", all.busiestHour())},
		{"after hours", percent(all.late, all.total)},
		{"weekends", percent(all.weekend, all.total)},
		{"time zones", zones(card.Zones)},
	}, width) + "\n\n")

	labels := []rune(strings.Repeat(" ", gutter+24*cw))
	for h := 0; h < 24; h += 3 {
		copy(labels[gutter+h*cw:], []rune(fmt.Sprintf("%02d", h)))
	}
	b.WriteString(t.dim.Render(strings.TrimRight(string(labels), " ")) + "\n")
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
		}
		if totals {
			b.WriteString(t.dim.Render(padLeft(fmt.Sprint(total), punchTotalWidth)))
		}
		b.WriteString("\n")
	}
	b.WriteString(strings.Repeat(" ", gutter) + t.accent.Render(strings.TrimRight(all.spark(cw), " ")) + "\n")

	scale := t.dim.Render(strings.Repeat(" ", gutter) + "Less ")
	for l := 0; l <= 4; l++ {
		scale += lipgloss.NewStyle().Foreground(t.cellColor(len(authorHues), l)).Render(punchGlyphs[l]) + " "
	}
	scale += t.dim.Render("More")
	b.WriteString("\n" + spread(scale, t.dim.Render("on each author's own clock "), width) + "\n\n")
	b.WriteString(m.rhythmTable(width))
	return indent(b.String(), max((m.width-width)/2, 0))
}

// zones names the most common UTC offset of the commits, such as "UTC+9"
// or "UTC+5:30", and how many offsets there are when more than one.
func zones(byOffset map[int]int) string {
	main, first := 0, true
	for off, n := range byOffset {
		if first || n > byOffset[main] || n == byOffset[main] && off < main {
			main, first = off, false
		}
	}
	sign, off := "+", main
	if off < 0 {
		sign, off = "-", -off
	}
	name := fmt.Sprintf("UTC%s%d", sign, off/3600)
	if off%3600 != 0 {
		name += fmt.Sprintf(":%02d", off%3600/60)
	}
	if len(byOffset) <= 1 {
		return name
	}
	return fmt.Sprintf("%s · %d zones", name, len(byOffset))
}

// Lines of the hours view outside the author rhythm rows.
const hoursChrome = 22

// rhythmTable lists each author's hourly profile, busiest hour and shares
// of commits after hours and on weekends, busiest author first, in their
// colors. Authors without their own hue, or beyond the screen height, fold
// into one row; columns that leave too little room for names are left out.
func (m Model) rhythmTable(width int) string {
	t := m.theme
	card := m.punch
	rhythms := map[int]rhythm{}
	var ids []int
	for id, grid := range card.ByAuthor {
		ids = append(ids, id)
		rhythms[id] = newRhythm(grid)
	}
	sort.Slice(ids, func(i, j int) bool {
		if a, b := rhythms[ids[i]].total, rhythms[ids[j]].total; a != b {
			return a > b
		}
		return ids[i] < ids[j]
	})

	type row struct {
		id    int // -1 for the folded row
		label string
		r     rhythm
	}
	maxRows := max(m.height-hoursChrome, 2)
	var rows []row
	var folded [7][24]int
	others := 0
	for _, id := range ids {
		if (id < len(authorHues) || id == m.author) && len(rows) < maxRows-1 {
			rows = append(rows, row{id: id, label: m.data.Authors[id].Name, r: rhythms[id]})
			continue
		}
		others++
		for wd := range folded {
			for h := range folded[wd] {
				folded[wd][h] += card.ByAuthor[id][wd][h]
			}
		}
	}
	if others > 0 {
		rows = append(rows, row{id: -1, label: fmt.Sprintf("+%d %s", others, plural(others, "other", "others")), r: newRhythm(&folded)})
	}

	columns := []struct {
		title string
		cell  func(rhythm) string
	}{
		{"peak", func(r rhythm) string { return fmt.Sprintf("%02d:00", r.busiestHour()) }},
		{"after hours", func(r rhythm) string { return percent(r.late, r.total) }},
		{"weekends", func(r rhythm) string { return percent(r.weekend, r.total) }},
	}
	const minLabel = 10
	fixed := func(n int) int { // everything but the name
		w := gutter + 2 + 2 + 24
		for _, c := range columns[:n] {
			w += 3 + len(c.title)
		}
		return w
	}
	n := len(columns)
	for n > 0 && width-fixed(n) < minLabel {
		n--
	}
	labelWidth := len("author")
	for _, r := range rows {
		labelWidth = max(labelWidth, ansi.StringWidth(r.label))
	}
	labelWidth = min(labelWidth, max(width-fixed(n), minLabel))

	pad := strings.Repeat(" ", gutter)
	header := "  " + padRight("author", labelWidth) + "  " + padRight("hours 00–23", 24)
	for _, c := range columns[:n] {
		header += "   " + c.title
	}
	var b strings.Builder
	b.WriteString(pad + t.dim.Render(header) + "\n")
	for _, r := range rows {
		hue, style := r.id, t.text
		if r.id < 0 {
			hue, style = len(authorHues), t.dim
		} else if r.id == m.author {
			style = style.Bold(true).Underline(true)
		}
		color := lipgloss.NewStyle().Foreground(t.authorColor(hue))
		label := ansi.Truncate(r.label, labelWidth, "…")
		line := pad + color.Render(cellGlyph) + " " + style.Render(label) +
			strings.Repeat(" ", labelWidth-ansi.StringWidth(label)) + "  " + color.Render(r.r.spark(1))
		for _, c := range columns[:n] {
			line += "   " + t.text.Render(padLeft(c.cell(r.r), len(c.title)))
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
