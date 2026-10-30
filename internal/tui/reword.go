package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ZhiWei-Ou/git-inktrace/internal/gitlog"
	"github.com/ZhiWei-Ou/git-inktrace/internal/stats"
)

// The git operations behind a reword; tests replace them.
var (
	checkReword        = gitlog.CheckReword
	editMessageCommand = gitlog.EditMessageCommand
	rewordCommit       = gitlog.Reword
)

// rewordConfirm is a reword waiting for the user to accept rewriting a
// commit that remote branches already hold.
type rewordConfirm struct {
	commit  *gitlog.GraphCommit
	remotes []string
}

type rewordCheckMsg struct {
	commit  *gitlog.GraphCommit
	remotes []string
	err     error
}

type rewordedMsg struct {
	commit *gitlog.GraphCommit
	hash   string // new hash of the reworded commit
	err    error
}

type dataMsg struct {
	data *stats.Data
	err  error
}

// startReword checks whether the selected commit can be reworded.
func (m *Model) startReword() tea.Cmd {
	g := &m.graph
	if !g.isCommit(g.cursor) {
		return nil
	}
	c, dir := g.lines[g.cursor].Commit, m.opts.Dir
	return func() tea.Msg {
		remotes, err := checkReword(dir, c.FullHash)
		return rewordCheckMsg{commit: c, remotes: remotes, err: err}
	}
}

// receiveRewordCheck opens the editor, or first asks for confirmation when
// the commit is already on a remote branch.
func (m *Model) receiveRewordCheck(msg rewordCheckMsg) tea.Cmd {
	g := &m.graph
	switch {
	case msg.err != nil:
		g.status = "Cannot reword " + msg.commit.Hash + ": " + msg.err.Error()
		return nil
	case len(msg.remotes) > 0:
		g.confirm = &rewordConfirm{commit: msg.commit, remotes: msg.remotes}
		return nil
	}
	return m.editMessage(msg.commit)
}

// confirmKey accepts the pending reword on y and cancels it on any other key.
func (m *Model) confirmKey(key string) tea.Cmd {
	g := &m.graph
	c := g.confirm.commit
	g.confirm = nil
	if key == "y" || key == "Y" {
		return m.editMessage(c)
	}
	g.status = "Reword cancelled"
	return nil
}

// editMessage hands the terminal to the editor on the commit's message, then
// rewords the commit with what the editor saved.
func (m *Model) editMessage(c *gitlog.GraphCommit) tea.Cmd {
	dir := m.opts.Dir
	cmd, path, err := editMessageCommand(dir, c.FullHash)
	if err != nil {
		m.graph.status = "Cannot edit " + c.Hash + ": " + err.Error()
		return nil
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return rewordedMsg{commit: c, err: fmt.Errorf("editor: %w", err)}
		}
		message, err := os.ReadFile(path)
		if err != nil {
			return rewordedMsg{commit: c, err: err}
		}
		hash, err := rewordCommit(dir, c.FullHash, string(message))
		return rewordedMsg{commit: c, hash: hash, err: err}
	})
}

// receiveReworded reports the reword and, when history changed, reloads the
// graph with the reworded commit selected, and the statistics.
func (m *Model) receiveReworded(msg rewordedMsg) tea.Cmd {
	g := &m.graph
	switch {
	case errors.Is(msg.err, gitlog.ErrMessageUnchanged):
		g.status = "Message unchanged, " + msg.commit.Hash + " kept"
		return nil
	case errors.Is(msg.err, gitlog.ErrMessageEmpty):
		g.status = "Empty message, " + msg.commit.Hash + " kept"
		return nil
	case msg.err != nil:
		g.status = "Reword failed: " + msg.err.Error()
		return nil
	}
	g.status = "Reworded " + msg.commit.Hash + " → " + shortHash(msg.hash)
	g.focus = msg.hash
	g.seq++ // drop pages read before the rewrite
	g.loaded, g.loading = false, false
	return tea.Batch(m.loadGraph(), m.reloadData())
}

// reloadData reads the history again for the heatmap, ranking and punch card.
func (m Model) reloadData() tea.Cmd {
	dir, all := m.opts.Dir, m.opts.All
	return func() tea.Msg {
		commits, err := gitlog.Read(dir, gitlog.Options{All: all})
		if err != nil {
			return dataMsg{err: err}
		}
		tracked, err := gitlog.Tracked(dir)
		if err != nil {
			return dataMsg{err: err}
		}
		return dataMsg{data: stats.New(commits, tracked)}
	}
}

func (m *Model) receiveData(msg dataMsg) {
	if msg.err != nil {
		m.graph.status = "Cannot reload history: " + msg.err.Error()
		return
	}
	m.data = msg.data
	m.refresh()
}

// confirmPrompt asks whether to rewrite a commit already on remote branches.
func (m Model) confirmPrompt() string {
	t := m.theme
	c := m.graph.confirm
	remotes := c.remotes
	more := ""
	if len(remotes) > 2 {
		remotes, more = remotes[:2], fmt.Sprintf(" and %d more", len(c.remotes)-2)
	}
	return " " + t.warn.Render("! "+c.commit.Hash+" is already on "+strings.Join(remotes, ", ")+more) +
		t.text.Render(", rewording it needs a force push. Continue?") + t.accent.Render(" [y/N]")
}

func shortHash(h string) string {
	return h[:min(len(h), 7)]
}
