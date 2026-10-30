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
	switch {
	case m.helpOpen:
		b.WriteString(m.helpPage())
	case m.inspect.open:
		b.WriteString(m.inspectView())
	case m.view == HeatmapView:
		b.WriteString(m.heatmap())
	case m.view == FilesView:
		b.WriteString(m.fileTable())
	case m.view == GraphView:
		b.WriteString(m.graphView())
	case m.view == HoursView:
		b.WriteString(m.punchCard())
	}
	b.WriteString("\n" + m.footer())
	return b.String()
}

func (m Model) header() string {
	t := m.theme
	left := " " + t.title.Render("git-inktrace") + "  " + t.text.Render(m.opts.Repo)
	if m.opts.Branch != "" {
		left += t.dim.Render(" · " + m.opts.Branch)
	}
	if m.view == GraphView {
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
	for i, name := range []string{"Heatmap", "Files", "Graph", "Hours"} {
		if View(i) == m.view {
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
	if m.view == GraphView {
		right = who + " "
	}
	return spread(" "+strings.Join(parts, " "), right, m.width)
}

// heatmap draws the calendar with its scale and author legend as one block,
// centered in the terminal.
func (m Model) heatmap() string {
	t := m.theme
	width := gutter + len(m.cal.Weeks)*cellWidth
	var b strings.Builder

	b.WriteString(m.overview(width) + "\n\n")
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
	b.WriteString("\n" + spread(scale, t.dim.Render(fmt.Sprintf("%d active %s ", active, plural(active, "day", "days"))), width) + "\n\n")
	b.WriteString(m.authorLegend(width))
	return indent(b.String(), max((m.width-width)/2, 0))
}

// indent shifts every non-empty line of s right by n columns.
func indent(s string, n int) string {
	if n == 0 {
		return s
	}
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
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
const heatmapChrome = 20

// overview sums up the repository above the heatmap.
func (m Model) overview(width int) string {
	created := m.opts.Created
	if created.IsZero() {
		created = m.data.Earliest()
	}
	return m.statRow([][2]string{
		{"created", formatDate(created)},
		{"age", age(created, time.Now())},
		{"last commit", ago(m.data.Latest(), today())},
		{"commits", thousands(m.data.Len())},
		{"authors", thousands(len(m.data.Authors))},
	}, width)
}

// statRow lays out titled values in columns spread across width with equal
// space around each, every value centered under its dim title. Columns
// that do not fit are left out from the right.
func (m Model) statRow(cells [][2]string, width int) string {
	t := m.theme
	const gap = 4 // least space between neighboring cells
	area := width - gutter
	widths := make([]int, len(cells))
	for i, c := range cells {
		widths[i] = max(len(c[0]), lipgloss.Width(c[1]))
	}
	used := 0
	n := 0
	for ; n < len(cells) && used+widths[n]+gap <= area; n++ {
		used += widths[n] + gap
	}
	titles, values := strings.Repeat(" ", gutter), strings.Repeat(" ", gutter)
	for i, c := range cells[:n] {
		// Share the spare columns out evenly, the remainder to the first slots.
		slot := widths[i] + (area-used+n*gap)/n
		if i < (area-used+n*gap)%n {
			slot++
		}
		center := func(s string) string {
			left := (slot - lipgloss.Width(s)) / 2
			return strings.Repeat(" ", left) + s + strings.Repeat(" ", slot-left-lipgloss.Width(s))
		}
		titles += t.dim.Render(center(c[0]))
		values += t.text.Bold(true).Render(center(c[1]))
	}
	return titles + "\n" + values
}

// age is the time from one date to another in at most two calendar units,
// such as "2 years 3 months", "10 months 9 days" or "12 days".
func age(from, to time.Time) string {
	from, to = midnight(from), midnight(to)
	if !to.After(from) {
		return "0 days"
	}
	// Whole months first, a month on from the 31st landing on the last day
	// of a shorter month, then the days left.
	addMonths := func(n int) time.Time {
		last := time.Date(from.Year(), from.Month()+time.Month(n)+1, 0, 0, 0, 0, 0, time.Local).Day()
		return time.Date(from.Year(), from.Month()+time.Month(n), min(from.Day(), last), 0, 0, 0, 0, time.Local)
	}
	months := (to.Year()-from.Year())*12 + int(to.Month()) - int(from.Month())
	if addMonths(months).After(to) {
		months--
	}
	y, mo := months/12, months%12
	d := int(to.Sub(addMonths(months)).Hours()/24 + 0.5)
	var parts []string
	for _, u := range []struct {
		n         int
		one, many string
	}{{y, "year", "years"}, {mo, "month", "months"}, {d, "day", "days"}} {
		if u.n > 0 && len(parts) < 2 {
			parts = append(parts, fmt.Sprintf("%d %s", u.n, plural(u.n, u.one, u.many)))
		} else if len(parts) > 0 {
			break // keep the two units adjacent, e.g. no "1 year 4 days"
		}
	}
	return strings.Join(parts, " ")
}

// ago describes a past date relative to today.
func ago(t, today time.Time) string {
	switch days := int(today.Sub(midnight(t)).Hours()/24 + 0.5); {
	case days <= 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days < 30:
		return fmt.Sprintf("%d days ago", days)
	}
	return formatDate(t)
}

// thousands formats n with comma separators.
func thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// authorLegend is a table of authors in their colors with their commits and
// share in the calendar month, the window and the whole history, busiest
// first. A column is left out when it would repeat the broader one next to it.
// Authors without their own hue, or beyond the screen height, fold into one
// row. Author labels are truncated to fit the legend in width columns.
func (m Model) authorLegend(width int) string {
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
	labelWidth = min(labelWidth, max(width-gutter-2-len(cols)*(numWidth+3), 8))

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
		line := row(
			t.dim.Render(fmt.Sprint(i+1)),
			t.text.Bold(true).Render(fmt.Sprint(f.Commits)),
			bar,
			addedStyle.Render(added),
			deletedStyle.Render(deleted),
			author,
			m.filePath(f, pathWidth),
		)
		if i == m.cursor {
			line = underline(line)
		}
		b.WriteString(line + "\n")
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
