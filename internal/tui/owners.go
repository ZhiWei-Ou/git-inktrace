package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ZhiWei-Ou/git-inktrace/internal/stats"
)

// ownersState is the ownership treemap: the directory shown, the selected
// tile and what tile areas measure.
type ownersState struct {
	root   *ownerNode
	dir    string // path of the directory shown, "" for the repository root
	cursor int    // selected tile, in order of size
	lines  bool   // area by lines changed instead of file changes
}

// ownerNode is a file or directory of the work tree with the changes made
// to it in the window. A directory chain with one subdirectory each, such
// as src/main/java, is one node.
type ownerNode struct {
	name     string // relative to the parent
	path     string
	dir      bool
	children []*ownerNode // largest first
	changes  int          // commits touching each file, summed
	added    int          // lines
	deleted  int
	byAuthor map[int]int // file changes per author
	file     *stats.FileStat
}

// buildOwners groups the files still in the work tree into directories.
func buildOwners(files []stats.FileStat) *ownerNode {
	root := &ownerNode{dir: true, byAuthor: map[int]int{}}
	index := map[string]*ownerNode{"": root}
	for i := range files {
		f := &files[i]
		if f.Gone {
			continue
		}
		node := root
		parts := strings.Split(f.Path, "/")
		for j, part := range parts {
			p := strings.Join(parts[:j+1], "/")
			child := index[p]
			if child == nil {
				child = &ownerNode{name: part, path: p, dir: j < len(parts)-1, byAuthor: map[int]int{}}
				index[p] = child
				node.children = append(node.children, child)
			}
			node = child
		}
		node.file = f
		for _, n := range ancestors(index, f.Path) {
			n.changes += f.Commits
			n.added += f.Added
			n.deleted += f.Deleted
			for a, c := range f.ByAuthor {
				n.byAuthor[a] += c
			}
		}
	}
	root.compress()
	return root
}

// ancestors returns the nodes from the root down to path itself.
func ancestors(index map[string]*ownerNode, p string) []*ownerNode {
	out := []*ownerNode{index[""]}
	for i, r := range p {
		if r == '/' {
			out = append(out, index[p[:i]])
		}
	}
	return append(out, index[p])
}

// compress joins each directory that holds a single directory with it.
func (n *ownerNode) compress() {
	for _, c := range n.children {
		for c.dir && len(c.children) == 1 && c.children[0].dir {
			only := c.children[0]
			c.name, c.path, c.children = c.name+"/"+only.name, only.path, only.children
		}
		c.compress()
	}
}

// parent returns the directory holding child, or nil.
func (n *ownerNode) parent(child *ownerNode) *ownerNode {
	for _, c := range n.children {
		if c == child {
			return n
		}
		if c.dir && strings.HasPrefix(child.path, c.path+"/") {
			return c.parent(child)
		}
	}
	return nil
}

// find returns the directory at p, or nil.
func (n *ownerNode) find(p string) *ownerNode {
	if n.path == p {
		return n
	}
	for _, c := range n.children {
		if c.dir && (c.path == p || strings.HasPrefix(p, c.path+"/")) {
			return c.find(p)
		}
	}
	return nil
}

func (n *ownerNode) weight(lines bool) int {
	if lines {
		return n.added + n.deleted
	}
	return n.changes
}

// owner returns the author with the most file changes, the share they
// hold, and the bus factor: how few authors made over half of the changes.
func (n *ownerNode) owner() (top int, share float64, bus int) {
	type count struct{ author, n int }
	var counts []count
	for a, c := range n.byAuthor {
		counts = append(counts, count{a, c})
	}
	if len(counts) == 0 || n.changes == 0 {
		return -1, 0, 0
	}
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].n != counts[j].n {
			return counts[i].n > counts[j].n
		}
		return counts[i].author < counts[j].author
	})
	sum := 0
	for _, c := range counts {
		sum += c.n
		bus++
		if 2*sum > n.changes {
			break
		}
	}
	return counts[0].author, float64(counts[0].n) / float64(n.changes), bus
}

// tile is a laid out child of the shown directory; node is nil for the
// tile folding the children too small to draw.
type tile struct {
	node           *ownerNode
	folded, weight int
	x0, y0, x1, y1 int // cells, end exclusive
}

// Smallest tile worth a label, in cells.
const minTileWidth, minTileHeight = 6, 2

// ownersTiles lays out the shown directory's children in a w by h cell map.
// Children too small to draw fold into one tile at the end.
func (m Model) ownersTiles(dir *ownerNode, w, h int) []tile {
	s := m.owners
	var tiles []tile
	total := 0
	for _, c := range dir.children {
		if wt := c.weight(s.lines); wt > 0 {
			tiles = append(tiles, tile{node: c, weight: wt})
			total += wt
		}
	}
	bySize := func() {
		sort.SliceStable(tiles, func(i, j int) bool { return tiles[i].weight > tiles[j].weight })
	}
	bySize()
	if total == 0 || w < minTileWidth || h < minTileHeight {
		return nil
	}
	// Keep as many of the largest children as get a readable tile, folding
	// the rest into one.
	children := tiles
	// Children below a minimal tile's share of the map cannot get one.
	minShare := float64(minTileWidth*minTileHeight) / float64(w*h)
	start := len(children)
	for start > 1 && float64(children[start-1].weight)/float64(total) < minShare {
		start--
	}
	for keep := start; keep >= 1; keep-- {
		tiles = append([]tile(nil), children[:keep]...)
		if keep < len(children) {
			rest := tile{folded: len(children) - keep}
			for _, t := range children[keep:] {
				rest.weight += t.weight
			}
			tiles = append(tiles, rest)
			bySize()
		}
		if layoutTiles(tiles, w, h) || keep == 1 {
			break
		}
	}
	return tiles
}

// layoutTiles places tiles, largest first, in a w by h cell map and
// reports whether every tile of a child is big enough for a label.
func layoutTiles(tiles []tile, w, h int) bool {
	// Lay out in square units: a cell is about twice as tall as it is wide.
	weights := make([]float64, len(tiles))
	for i, t := range tiles {
		weights[i] = float64(t.weight)
	}
	readable := true
	for i, r := range squarify(weights, rect{0, 0, float64(w) / 2, float64(h)}) {
		t := &tiles[i]
		t.x0, t.x1 = int(math.Round(r.x*2)), int(math.Round((r.x+r.w)*2))
		t.y0, t.y1 = int(math.Round(r.y)), int(math.Round(r.y+r.h))
		if t.node != nil && (t.x1-t.x0 < minTileWidth || t.y1-t.y0 < minTileHeight) {
			readable = false
		}
	}
	return readable
}

type rect struct{ x, y, w, h float64 }

// squarify lays weights, largest first, out in r with tiles as close to
// square as it can (Bruls, Huizing and van Wijk), filling rows along the
// shorter side.
func squarify(weights []float64, r rect) []rect {
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	areas := make([]float64, len(weights))
	for i, w := range weights {
		areas[i] = w / sum * r.w * r.h
	}
	worst := func(row []float64, side float64) float64 {
		s, hi, lo := 0.0, 0.0, math.Inf(1)
		for _, a := range row {
			s, hi, lo = s+a, max(hi, a), min(lo, a)
		}
		return max(side*side*hi/(s*s), s*s/(side*side*lo))
	}
	var out []rect
	for len(areas) > 0 {
		side := min(r.w, r.h)
		n := 1
		for n < len(areas) && worst(areas[:n+1], side) <= worst(areas[:n], side) {
			n++
		}
		row, s := areas[:n], 0.0
		for _, a := range row {
			s += a
		}
		if r.w >= r.h { // a column at the left
			cw, y := s/r.h, r.y
			for _, a := range row {
				out = append(out, rect{r.x, y, cw, a / cw})
				y += a / cw
			}
			r.x, r.w = r.x+cw, r.w-cw
		} else { // a row at the top
			rh, x := s/r.w, r.x
			for _, a := range row {
				out = append(out, rect{x, r.y, a / rh, rh})
				x += a / rh
			}
			r.y, r.h = r.y+rh, r.h-rh
		}
		areas = areas[n:]
	}
	return out
}

// Lines of the owners view outside the map: header, tabs, blank, path,
// details, blank and help.
const ownersChrome = 7

func (m Model) ownersMapSize() (int, int) {
	return max(m.width-2, 0), max(m.height-ownersChrome, 0)
}

// shownDir is the directory on the map, falling back to the root when the
// window or filter left it empty.
func (m Model) shownDir() *ownerNode {
	if m.owners.root == nil {
		return nil
	}
	if d := m.owners.root.find(m.owners.dir); d != nil {
		return d
	}
	return m.owners.root
}

// refreshOwners rebuilds the tree for the window.
func (m *Model) refreshOwners() {
	if m.view != OwnersView {
		return
	}
	s := &m.owners
	s.root = buildOwners(m.data.FileRank(m.cal.Start, m.cal.End, 0))
	if d := m.shownDir(); d.path != s.dir {
		s.dir, s.cursor = d.path, 0
	}
	if n := len(m.selectableTiles()); s.cursor >= n {
		s.cursor = max(n-1, 0)
	}
}

// selectableTiles are the tiles of real children, largest first.
func (m Model) selectableTiles() []tile {
	dir := m.shownDir()
	if dir == nil {
		return nil
	}
	var out []tile
	w, h := m.ownersMapSize()
	for _, t := range m.ownersTiles(dir, w, h) {
		if t.node != nil {
			out = append(out, t)
		}
	}
	return out
}

func (m Model) selectedTile() *tile {
	tiles := m.selectableTiles()
	if m.owners.cursor < len(tiles) {
		return &tiles[m.owners.cursor]
	}
	return nil
}

func (m *Model) ownersKey(key string) {
	s := &m.owners
	n := len(m.selectableTiles())
	switch key {
	case "up", "k":
		s.cursor = max(s.cursor-1, 0)
	case "down", "j":
		s.cursor = min(s.cursor+1, max(n-1, 0))
	case "home", "g":
		s.cursor = 0
	case "end", "G":
		s.cursor = max(n-1, 0)
	case "esc", "backspace":
		// Up to the parent, selecting the directory just left.
		if dir := m.shownDir(); dir != nil && dir != s.root {
			s.dir, s.cursor = s.root.parent(dir).path, 0
			for i, t := range m.selectableTiles() {
				if t.node == dir {
					s.cursor = i
				}
			}
		}
	case "w":
		s.lines = !s.lines
		s.cursor = 0
	}
}

// ownersEnter opens the selected directory, or the commits of the
// selected file.
func (m *Model) ownersEnter() {
	t := m.selectedTile()
	if t == nil {
		return
	}
	if t.node.dir {
		m.owners.dir, m.owners.cursor = t.node.path, 0
		return
	}
	m.inspect = inspectState{open: true, path: t.node.path,
		commits: m.data.FileCommits(t.node.path, m.cal.Start, m.cal.End)}
}

func (m Model) ownersView() string {
	t := m.theme
	if status := m.indexStatus(); status != "" && m.owners.root != nil && len(m.owners.root.children) == 0 {
		return " " + status + "\n"
	}
	dir := m.shownDir()
	if dir == nil || len(dir.children) == 0 {
		return " " + t.dim.Render("No changes to files in the work tree in this range") + "\n"
	}
	w, h := m.ownersMapSize()
	tiles := m.ownersTiles(dir, w, h)
	selected := m.selectedTile()

	where := m.opts.Repo
	if dir.path != "" {
		where += "/" + dir.path
	}
	measure := "area: file changes"
	if m.owners.lines {
		measure = "area: lines changed"
	}
	var b strings.Builder
	b.WriteString(spread(" "+t.text.Bold(true).Render(where+"/"), t.dim.Render(measure)+" ", m.width) + "\n")

	// Paint each tile into a cell grid, leaving a gap at its right and
	// bottom edges so neighbors of one color stay apart.
	type cell struct {
		r    string
		tile int // -1 for the gap
	}
	grid := make([][]cell, h)
	for y := range grid {
		grid[y] = make([]cell, w)
		for x := range grid[y] {
			grid[y][x] = cell{" ", -1}
		}
	}
	styles := make([]lipgloss.Style, len(tiles))
	for i, tl := range tiles {
		x1, y1 := min(tl.x1, w)-1, min(tl.y1, h)-1
		if tl.x1 >= w {
			x1 = w // no gap at the map's right edge
		}
		if tl.y1 >= h {
			y1 = h
		}
		if x1 <= tl.x0 || y1 <= tl.y0 {
			continue
		}
		isSel := selected != nil && tl.node != nil && tl.node == selected.node
		styles[i] = m.tileStyle(tl, isSel)
		for y := tl.y0; y < y1; y++ {
			for x := tl.x0; x < x1; x++ {
				grid[y][x] = cell{" ", i}
			}
		}
		// The label sits inside the tile, and inside the selected tile's
		// border: a box when it has the rows, else just its sides.
		lx, ly, lx1, ly1 := tl.x0+1, tl.y0, x1, y1
		if isSel {
			lx1--
			for y := tl.y0; y < y1; y++ {
				grid[y][tl.x0].r, grid[y][x1-1].r = "│", "│"
			}
			if y1-tl.y0 >= 3 {
				ly, ly1 = tl.y0+1, y1-1
				for x := tl.x0 + 1; x < x1-1; x++ {
					grid[tl.y0][x].r, grid[y1-1][x].r = "─", "─"
				}
				grid[tl.y0][tl.x0].r, grid[tl.y0][x1-1].r = "╭", "╮"
				grid[y1-1][tl.x0].r, grid[y1-1][x1-1].r = "╰", "╯"
			}
		}
		for j, line := range m.tileLabel(tl, lx1-lx) {
			if ly+j >= ly1 {
				break
			}
			for k, r := range []rune(line) {
				if x := lx + k; x < lx1 {
					grid[ly+j][x].r = string(r)
				}
			}
		}
	}
	for _, row := range grid {
		var line strings.Builder
		line.WriteString(" ")
		for x := 0; x < len(row); {
			end := x
			var run strings.Builder
			for end < len(row) && row[end].tile == row[x].tile {
				run.WriteString(row[end].r)
				end++
			}
			if row[x].tile < 0 {
				line.WriteString(run.String())
			} else {
				line.WriteString(styles[row[x].tile].Render(run.String()))
			}
			x = end
		}
		b.WriteString(line.String() + "\n")
	}
	b.WriteString(m.ownersDetails(selected) + "\n")
	return b.String()
}

// tileStyle fills a tile with its owner's color, deeper the larger the
// owner's share, and picks a readable text color on it.
func (m Model) tileStyle(tl tile, selected bool) lipgloss.Style {
	t := m.theme
	hue, level := len(authorHues), 1
	if tl.node != nil {
		top, share, _ := tl.node.owner()
		hue = min(top, len(authorHues))
		level = min(1+int(share*4), 4)
	}
	bg := t.cellColor(hue, level)
	style := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(contrastText(string(bg))))
	if selected {
		style = style.Bold(true)
	}
	return style
}

// tileLabel is the text inside a tile: its name, then its owner and share,
// then its size, as far as the tile fits them.
func (m Model) tileLabel(tl tile, width int) []string {
	if width < 1 {
		return nil
	}
	fit := func(s string) string { return ansi.Truncate(s, width, "…") }
	if tl.node == nil {
		return []string{fit(fmt.Sprintf("+%d more", tl.folded))}
	}
	n := tl.node
	name := n.name
	if n.dir {
		name += "/"
	}
	top, share, _ := n.owner()
	lines := []string{fit(name)}
	if top >= 0 {
		lines = append(lines, fit(fmt.Sprintf("%s %.0f%%", m.data.Authors[top].Name, share*100)))
	}
	return append(lines, fit(thousands(n.weight(m.owners.lines))))
}

// ownersDetails describes the selected tile below the map.
func (m Model) ownersDetails(sel *tile) string {
	t := m.theme
	if sel == nil {
		return ""
	}
	n := sel.node
	name := n.path
	if n.dir {
		name += "/"
	}
	top, share, bus := n.owner()
	parts := []string{
		t.text.Render(thousands(n.changes) + " " + plural(n.changes, "change", "changes")),
		addedStyle.Render("+"+human(n.added)) + " " + deletedStyle.Render("-"+human(n.deleted)),
		t.text.Render(fmt.Sprintf("%d %s", len(n.byAuthor), plural(len(n.byAuthor), "author", "authors"))),
	}
	if top >= 0 {
		color := lipgloss.NewStyle().Foreground(t.authorColor(top))
		parts = append(parts, color.Render(cellGlyph)+" "+t.text.Render(fmt.Sprintf("%s %.0f%%", m.data.Authors[top].Name, share*100)))
	}
	switch {
	case len(n.byAuthor) == 1:
		parts = append(parts, t.warn.Render("sole author"))
	case bus == 1:
		parts = append(parts, t.warn.Render("bus factor 1"))
	default:
		parts = append(parts, t.dim.Render(fmt.Sprintf("bus factor %d", bus)))
	}
	line := " " + t.text.Bold(true).Render(name) + t.dim.Render("  ·  ") + strings.Join(parts, t.dim.Render("  ·  "))
	return ansi.Truncate(line, max(m.width-1, 1), "…")
}

// contrastText returns dark or light text for a #rrggbb background.
func contrastText(bg string) string {
	var r, g, b int
	fmt.Sscanf(bg, "#%02x%02x%02x", &r, &g, &b)
	if 0.299*float64(r)+0.587*float64(g)+0.114*float64(b) > 150 {
		return "#0d1117"
	}
	return "#f0f6fc"
}
