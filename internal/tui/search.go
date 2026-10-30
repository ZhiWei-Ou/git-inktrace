package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ZhiWei-Ou/git-inktrace/internal/gitlog"
)

// graphQuery is a parsed graph search. Plain words match the commit message;
// "@commit", "@author" and "@email" make the words after them match the hash
// prefix, the author name or the email instead. Every part must match, case
// insensitively, e.g. "fix parser @author bob @email x.io".
type graphQuery struct {
	message, commit, author, email string
}

// queryFields are the search keywords, in the order tab completes them.
var queryFields = []string{"@commit", "@author", "@email"}

func parseQuery(s string) graphQuery {
	var q graphQuery
	parts := map[string][]string{}
	field := ""
	for _, w := range strings.Fields(strings.ToLower(s)) {
		switch w {
		case "@commit", "@author", "@email":
			field = w
		default:
			parts[field] = append(parts[field], w)
		}
	}
	q.message = strings.Join(parts[""], " ")
	q.commit = strings.Join(parts["@commit"], " ")
	q.author = strings.Join(parts["@author"], " ")
	q.email = strings.Join(parts["@email"], " ")
	return q
}

func (q graphQuery) empty() bool { return q == graphQuery{} }

func (q graphQuery) match(c *gitlog.GraphCommit) bool {
	return c != nil &&
		strings.Contains(strings.ToLower(c.Subject), q.message) &&
		strings.HasPrefix(strings.ToLower(c.FullHash), q.commit) &&
		strings.Contains(strings.ToLower(c.Author), q.author) &&
		strings.Contains(strings.ToLower(c.Email), q.email)
}

// searchKey edits the search prompt, highlighting matches as the text
// changes; tab completes a keyword, enter searches and esc closes the prompt
// along with the highlight.
func (m *Model) searchKey(msg tea.KeyMsg) tea.Cmd {
	g := &m.graph
	defer func() {
		if g.typing {
			g.match = parseQuery(g.input)
		}
	}()
	switch msg.Type {
	case tea.KeyRunes:
		g.input += string(msg.Runes)
	case tea.KeySpace:
		g.input += " "
	case tea.KeyBackspace:
		if r := []rune(g.input); len(r) > 0 {
			g.input = string(r[:len(r)-1])
		}
	case tea.KeyCtrlU:
		g.input = ""
	case tea.KeyTab:
		g.input = completeField(g.input)
	case tea.KeyEsc:
		g.typing = false
		g.clearSearch()
	case tea.KeyEnter:
		g.typing = false
		g.query = strings.TrimSpace(g.input)
		g.match = parseQuery(g.query)
		if g.match.empty() {
			g.clearSearch() // an empty search clears the highlight
			return nil
		}
		return m.jumpMatch(g.cursor, 1)
	}
	return nil
}

// clearSearch drops the last search, its highlight and its match count.
func (g *graphState) clearSearch() {
	g.query, g.match, g.status = "", graphQuery{}, ""
}

// completeField completes a keyword prefix such as "@com" at the end of s.
func completeField(s string) string {
	start := strings.LastIndex(s, " ") + 1
	word := strings.ToLower(s[start:])
	if !strings.HasPrefix(word, "@") {
		return s
	}
	var found []string
	for _, f := range queryFields {
		if strings.HasPrefix(f, word) {
			found = append(found, f)
		}
	}
	if len(found) != 1 {
		return s
	}
	return s[:start] + found[0] + " "
}

// jumpMatch moves the cursor to the first commit matching the last search
// from line i in direction dir (+1 or -1). Past the last loaded match it
// stays put and loads older history.
func (m *Model) jumpMatch(i, dir int) tea.Cmd {
	g := &m.graph
	if g.query == "" {
		return nil
	}
	for ; i >= 0 && i < len(g.lines); i += dir {
		if g.match.match(g.lines[i].Commit) {
			g.cursor = i
			nth, total := 0, 0
			for j, l := range g.lines {
				if g.match.match(l.Commit) {
					total++
					if j <= i {
						nth++
					}
				}
			}
			g.status = fmt.Sprintf("/%s · match %d of %d loaded", g.query, nth, total)
			m.clampGraph()
			return m.loadMoreGraph()
		}
	}
	switch {
	case dir < 0:
		g.status = "No earlier match for /" + g.query
	case g.done:
		g.status = "No later match for /" + g.query
	default:
		g.status = "No later match loaded for /" + g.query + " · loading older history"
		if !g.loading {
			g.limit += graphPage
			g.loaded = false
			return m.loadGraph()
		}
	}
	return nil
}
