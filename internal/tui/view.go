package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ZhiWei-Ou/git-inktrace/internal/stats"
)

const (
	gutter    = 5 // left margin plus weekday labels
	cellWidth = 2
	cellGlyph = "■"

	// Lines around the file table: header, tabs, blank, column titles, blank, help.
	filesChrome = 6
)

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.header() + "\n")
	b.WriteString(m.tabs() + "\n\n")
	switch m.view {
	case heatmapView:
		b.WriteString(m.heatmap())
	case filesView:
		b.WriteString(m.fileTable())
	case graphView:
		b.WriteString(m.graphView())
	}
	b.WriteString("\n" + m.help())
	return b.String()
}

func (m Model) header() string {
	t := m.theme
	left := " " + t.title.Render("git-inktrace") + "  " + t.text.Render(m.opts.Repo)
	if m.opts.Branch != "" {
		left += t.dim.Render(" · " + m.opts.Branch)
	}
	if m.view == graphView {
		return spread(left, t.dim.Render(m.graphScope())+" ", m.width)
	}
	// Arrows show which way more history lies.
	back, fwd := "  ", "  "
	if m.hasEarlier() {
		back = "← "
	}
	if m.end.Before(defaultEnd(m.data)) {
		fwd = " →"
	}
	right := t.accent.Render(back) + t.dim.Render(formatDate(m.cal.Start)+" – "+formatDate(m.cal.End)) +
		t.accent.Render(fwd) + " "
	return spread(left, right, m.width)
}

func (m Model) tabs() string {
	t := m.theme
	var parts []string
	for i, name := range []string{"Heatmap", "Files", "Graph"} {
		if view(i) == m.view {
			parts = append(parts, t.activeTab.Render(name))
		} else {
			parts = append(parts, t.tab.Render(name))
		}
	}
	who := t.text.Render("all authors")
	if m.author != stats.AllAuthors {
		who = lipgloss.NewStyle().Foreground(t.authorColor(m.author)).Bold(true).
			Render(cellGlyph + " " + m.data.Authors[m.author].Name)
	}
	right := t.dim.Render(fmt.Sprintf("%d commits · ", m.cal.Total)) + who + " "
	if m.view == graphView {
		right = who + " "
	}
	return spread(" "+strings.Join(parts, " "), right, m.width)
}

func (m Model) help() string {
	keys := "←→ year · t latest · a/A author · tab files · q quit"
	switch m.view {
	case filesView:
		keys = "↑↓ scroll · pgup/pgdn page · ←→ year · t latest · a/A author · tab graph · q quit"
	case graphView:
		keys = "↑↓ scroll · pgup/pgdn page · g/G top/bottom · b HEAD/all branches · a/A author · tab heatmap · q quit"
	}
	return " " + m.theme.dim.Render(ansi.Truncate(keys, max(m.width-2, 0), "…"))
}

func (m Model) heatmap() string {
	t := m.theme
	var b strings.Builder

	b.WriteString(m.monthLabels() + "\n")
	weekdays := [7]string{"", "Mon", "", "Wed", "", "Fri", ""}
	for wd := 0; wd < 7; wd++ {
		b.WriteString(" " + t.dim.Render(fmt.Sprintf("%-4s", weekdays[wd])))
		for w := range m.cal.Weeks {
			day := m.cal.Weeks[w][wd]
			if day.Future {
				b.WriteString(strings.Repeat(" ", cellWidth))
				continue
			}
			color := t.cellColor(day.Top, m.cal.Level(day.Total))
			b.WriteString(lipgloss.NewStyle().Foreground(color).Render(cellGlyph) + " ")
		}
		b.WriteString("\n")
	}

	// Intensity scale, drawn in the neutral hue.
	scale := t.dim.Render(strings.Repeat(" ", gutter) + "Less ")
	for l := 0; l <= 4; l++ {
		scale += lipgloss.NewStyle().Foreground(t.cellColor(len(authorHues), l)).Render(cellGlyph) + " "
	}
	scale += t.dim.Render("More")
	active := 0
	for _, w := range m.cal.Weeks {
		for _, d := range w {
			if d.Total > 0 {
				active++
			}
		}
	}
	b.WriteString("\n" + spread(scale, t.dim.Render(fmt.Sprintf("%d active %s ", active, plural(active, "day", "days"))), m.width) + "\n\n")
	b.WriteString(m.authorLegend())
	return b.String()
}

// monthLabels places a month name above the first week starting in that month.
func (m Model) monthLabels() string {
	type label struct {
		col  int
		text string
	}
	var labels []label
	for w, week := range m.cal.Weeks {
		first := week[0].Date
		if w > 0 && first.Month() == m.cal.Weeks[w-1][0].Date.Month() {
			continue
		}
		labels = append(labels, label{gutter + w*cellWidth, first.Format("Jan")})
	}
	// Like GitHub, drop the leading partial month when it would crowd the next one.
	if len(labels) > 1 && labels[0].col+len(labels[0].text) >= labels[1].col {
		labels = labels[1:]
	}
	line := []rune(strings.Repeat(" ", gutter+len(m.cal.Weeks)*cellWidth))
	next := 0 // first free column, so labels never overlap
	for _, l := range labels {
		if l.col < next || l.col+len(l.text) > len(line) {
			continue
		}
		copy(line[l.col:], []rune(l.text))
		next = l.col + len(l.text) + 1
	}
	return m.theme.dim.Render(strings.TrimRight(string(line), " "))
}

// Lines of the heatmap view outside the author legend rows.
const heatmapChrome = 17

// authorLegend is a table of authors in their colors with their commits and
// share in the calendar month, the window and the whole history, busiest
// first. A column is left out when it would repeat the broader one next to it.
// Authors without their own hue, or beyond the screen height, fold into one row.
func (m Model) authorLegend() string {
	t := m.theme
	all := make([]int, len(m.data.Authors))
	for id, a := range m.data.Authors {
		all[id] = a.Commits
	}
	type column struct {
		title  string
		counts []int // per author
		total  int
	}
	candidates := []column{
		{title: m.cal.End.Format("Jan 2006"), counts: m.monthShares},
		{title: "this window", counts: m.windowShares},
		{title: "all history", counts: all},
	}
	for i := range candidates {
		for _, n := range candidates[i].counts {
			candidates[i].total += n
		}
	}
	var cols []column
	for i, c := range candidates {
		if i == len(candidates)-1 || c.total != candidates[i+1].total {
			cols = append(cols, c)
		}
	}

	var ids []int
	for id, n := range all {
		if n > 0 {
			ids = append(ids, id)
		}
	}
	sort.SliceStable(ids, func(i, j int) bool {
		for _, c := range cols {
			if a, b := c.counts[ids[i]], c.counts[ids[j]]; a != b {
				return a > b
			}
		}
		return ids[i] < ids[j]
	})

	type row struct {
		id     int // -1 for the folded row
		label  string
		values []int // per column
		folded int
	}
	maxRows := max(m.height-heatmapChrome, 2)
	var rows []row
	others := row{id: -1, values: make([]int, len(cols))}
	for _, id := range ids {
		a := m.data.Authors[id]
		own := id < len(authorHues) || id == m.author
		if own && len(rows) < maxRows-1 {
			r := row{id: id, label: a.Name + " <" + a.Email + ">"}
			for _, c := range cols {
				r.values = append(r.values, c.counts[id])
			}
			rows = append(rows, r)
			continue
		}
		others.folded++
		for i, c := range cols {
			others.values[i] += c.counts[id]
		}
	}
	if others.folded > 0 {
		others.label = fmt.Sprintf("+%d %s", others.folded, plural(others.folded, "other", "others"))
		rows = append(rows, others)
	}

	const numWidth = 7 + 1 + 6 // count, space, percent
	labelWidth := 0
	for _, r := range rows {
		labelWidth = max(labelWidth, ansi.StringWidth(r.label))
	}
	labelWidth = min(labelWidth, max(m.width-gutter-2-len(cols)*(numWidth+3), 8))

	indent := strings.Repeat(" ", gutter)
	var b strings.Builder
	header := padRight("  author", labelWidth+2)
	for _, c := range cols {
		header += "   " + padLeft(c.title, numWidth)
	}
	b.WriteString(indent + t.dim.Render(header) + "\n")
	for _, r := range rows {
		label := ansi.Truncate(r.label, labelWidth, "…")
		style := t.text
		if r.id < 0 {
			style = t.dim
		} else if r.id == m.author {
			style = style.Bold(true).Underline(true)
		}
		hue := r.id
		if hue < 0 {
			hue = len(authorHues) // the shared "others" hue
		}
		glyph := lipgloss.NewStyle().Foreground(t.authorColor(hue)).Render(cellGlyph)
		line := indent + glyph + " " + style.Render(label) + strings.Repeat(" ", labelWidth-ansi.StringWidth(label))
		for i, c := range cols {
			line += "   " + m.shareCell(r.values[i], c.total)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// shareCell renders a commit count and its share of total.
func (m Model) shareCell(n, total int) string {
	if n == 0 {
		return m.theme.dim.Render(padLeft("–", 7) + " " + padLeft("", 6))
	}
	return m.theme.text.Render(padLeft(fmt.Sprint(n), 7)) + " " + m.theme.accent.Render(padLeft(percent(n, total), 6))
}

func (m Model) fileRows() int {
	return m.height - filesChrome - 1
}

func (m Model) fileTable() string {
	t := m.theme
	if len(m.files) == 0 {
		return " " + t.dim.Render("No file changes in this range") + "\n"
	}

	const barWidth = 12
	authorWidth := 16
	pathWidth := m.width - (1 + 4 + 2 + 7 + 1 + barWidth + 2 + 7 + 1 + 7 + 2 + authorWidth + 2)
	if pathWidth < 16 {
		authorWidth = max(authorWidth-(16-pathWidth), 2)
		pathWidth = 16
	}
	row := func(rank, commits, bar, added, deleted, author, path string) string {
		return " " + padLeft(rank, 4) + "  " + padLeft(commits, 7) + " " + padRight(bar, barWidth) + "  " +
			padLeft(added, 7) + " " + padLeft(deleted, 7) + "  " + padRight(author, authorWidth) + "  " + path
	}

	var b strings.Builder
	b.WriteString(t.dim.Render(row("#", "commits", "", "added", "deleted", "top author", "file")) + "\n")
	top := m.files[0].Commits
	end := min(m.scroll+m.fileRows(), len(m.files))
	for i := m.scroll; i < end; i++ {
		f := m.files[i]
		color := lipgloss.NewStyle().Foreground(t.authorColor(f.TopAuthor))
		bar := color.Render(strings.Repeat("█", max(f.Commits*barWidth/top, 1)))
		added, deleted := "+"+human(f.Added), "-"+human(f.Deleted)
		if f.Binary && f.Added == 0 && f.Deleted == 0 {
			added, deleted = "bin", ""
		}
		author := color.Render(cellGlyph) + " " + ansi.Truncate(m.data.Authors[f.TopAuthor].Name, authorWidth-2, "…")
		b.WriteString(row(
			t.dim.Render(fmt.Sprint(i+1)),
			t.text.Bold(true).Render(fmt.Sprint(f.Commits)),
			bar,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Render(added),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149")).Render(deleted),
			author,
			m.filePath(f, pathWidth),
		) + "\n")
	}
	if len(m.files) > m.fileRows() {
		b.WriteString(" " + t.dim.Render(fmt.Sprintf("%d–%d of %d", m.scroll+1, end, len(m.files))) + "\n")
	}
	return b.String()
}

// filePath renders a ranked path, dimmed and tagged when the file is gone.
func (m Model) filePath(f stats.FileStat, width int) string {
	t := m.theme
	if !f.Gone {
		return t.text.Render(truncatePath(f.Path, width))
	}
	const tag = " (deleted)"
	return t.dim.Strikethrough(true).Render(truncatePath(f.Path, max(width-len(tag), 4))) + t.dim.Render(tag)
}

// spread places left and right at the edges of a line of the given width.
func spread(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// wrap joins parts with double spaces, breaking lines before width.
func wrap(first string, parts []string, indent string, width int) string {
	var b strings.Builder
	line := first
	lineStart := true
	for _, p := range parts {
		if !lineStart && lipgloss.Width(line)+2+lipgloss.Width(p) > width {
			b.WriteString(line + "\n")
			line, lineStart = indent, true
		}
		if !lineStart {
			line += "  "
		}
		line += p
		lineStart = false
	}
	b.WriteString(line)
	return b.String()
}

func padLeft(s string, width int) string {
	return strings.Repeat(" ", max(width-lipgloss.Width(s), 0)) + s
}

func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0))
}

func truncatePath(p string, width int) string {
	if w := ansi.StringWidth(p); w > width {
		return ansi.TruncateLeft(p, w-width+1, "…")
	}
	return p
}

func human(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

// percent formats n as a share of total, keeping small non-zero shares visible.
func percent(n, total int) string {
	p := 100 * float64(n) / float64(total)
	if p > 0 && p < 0.1 {
		return "<0.1%"
	}
	return fmt.Sprintf("%.1f%%", p)
}

func formatDate(t time.Time) string { return t.Format("Jan 2, 2006") }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
