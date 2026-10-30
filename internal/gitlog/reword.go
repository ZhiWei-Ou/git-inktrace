package gitlog

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrMessageUnchanged and ErrMessageEmpty abort Reword without changes.
var (
	ErrMessageUnchanged = errors.New("message unchanged")
	ErrMessageEmpty     = errors.New("empty message")
)

// CheckReword reports whether the message of hash can be reworded: the
// commit must be reachable from HEAD and no merge, rebase or similar
// operation may be in progress. It returns the remote-tracking branches that
// already contain the commit, which a reword would make diverge.
func CheckReword(dir, hash string) ([]string, error) {
	for _, state := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD"} {
		path, err := gitPath(dir, state)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(path); err == nil {
			return nil, fmt.Errorf("finish the operation in progress first (%s)", state)
		}
	}
	if _, err := git(dir, "merge-base", "--is-ancestor", hash, "HEAD"); err != nil {
		return nil, fmt.Errorf("the commit is not on the current branch")
	}
	out, err := git(dir, "for-each-ref", "--contains", hash, "--format=%(refname:short)", "refs/remotes")
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// EditMessageCommand writes the message of hash to the file git uses for
// commit messages and returns that file with a command opening it in the
// editor git would use (GIT_EDITOR, core.editor, VISUAL, EDITOR, then vi).
func EditMessageCommand(dir, hash string) (*exec.Cmd, string, error) {
	c, err := readCommits(dir, []string{hash})
	if err != nil {
		return nil, "", err
	}
	path, err := gitPath(dir, "COMMIT_EDITMSG")
	if err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(path, []byte(c[0].message), 0o644); err != nil {
		return nil, "", err
	}
	editor, err := git(dir, "var", "GIT_EDITOR")
	if err != nil {
		return nil, "", err
	}
	// Like git, let the shell split the editor command and its arguments.
	cmd := exec.Command("sh", "-c", strings.TrimSpace(string(editor))+` "$@"`, "editor", path)
	cmd.Dir = dir
	return cmd, path, nil
}

// Reword replaces the message of hash, a commit reachable from HEAD, and
// recreates the commits between it and HEAD on top of it. Trees, authors and
// author dates stay as they were, so the work tree and index are untouched
// and no conflict can arise. Merges keep their shape. HEAD (or the branch it
// points to) moves to the rewritten tip, and only if it has not moved since
// the reword started; other branches keep the original commits.
//
// The message is cleaned up like `git commit` does without stripping
// comments. Reword returns the new hash of the reworded commit.
func Reword(dir, hash, message string) (string, error) {
	cleaned, err := gitInput(dir, message, nil, "stripspace")
	if err != nil {
		return "", err
	}
	if len(bytes.TrimSpace(cleaned)) == 0 {
		return "", ErrMessageEmpty
	}
	head, err := revParse(dir, "HEAD")
	if err != nil {
		return "", err
	}
	full, err := revParse(dir, hash+"^{commit}")
	if err != nil {
		return "", err
	}
	out, err := git(dir, "rev-list", "--topo-order", "--reverse", full+".."+head)
	if err != nil {
		return "", err
	}
	commits, err := readCommits(dir, append([]string{full}, strings.Fields(string(out))...))
	if err != nil {
		return "", err
	}
	if string(cleaned) == commits[0].message {
		return "", ErrMessageUnchanged
	}
	commits[0].message = string(cleaned)

	rewritten := map[string]string{} // old hash -> new hash
	for i, c := range commits {
		changed := i == 0
		for j, p := range c.parents {
			if n, ok := rewritten[p]; ok {
				c.parents[j], changed = n, true
			}
		}
		if !changed {
			continue // a merged-in commit that does not descend from hash
		}
		if rewritten[c.hash], err = commitTree(dir, c); err != nil {
			return "", err
		}
	}
	tip, ok := rewritten[head]
	if !ok {
		return "", fmt.Errorf("HEAD does not descend from %s", hash)
	}
	subject, _, _ := strings.Cut(commits[0].message, "\n")
	if _, err := git(dir, "update-ref", "-m", "reword (git-inktrace): "+subject, "HEAD", tip, head); err != nil {
		return "", err
	}
	return rewritten[full], nil
}

// rawCommit is the part of a commit object that Reword carries over.
type rawCommit struct {
	hash, tree string
	parents    []string
	author     string // "Name <email> timestamp zone"
	encoding   string // of the message, empty for UTF-8
	message    string
}

// readCommits reads commit objects in the given order with one git process.
func readCommits(dir string, hashes []string) ([]rawCommit, error) {
	out, err := gitInput(dir, strings.Join(hashes, "\n")+"\n", nil, "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	r := bufio.NewReader(bytes.NewReader(out))
	commits := make([]rawCommit, 0, len(hashes))
	for range hashes {
		header, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("cat-file: %w", err)
		}
		f := strings.Fields(header)
		if len(f) != 3 || f[1] != "commit" {
			return nil, fmt.Errorf("not a commit: %s", strings.TrimSpace(header))
		}
		size, err := strconv.Atoi(f[2])
		if err != nil {
			return nil, fmt.Errorf("cat-file: %q", header)
		}
		body := make([]byte, size+1) // the object and its trailing newline
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, fmt.Errorf("cat-file: %w", err)
		}
		c := rawCommit{hash: f[0]}
		headers, message, _ := strings.Cut(string(body[:size]), "\n\n")
		c.message = message
		for line := range strings.SplitSeq(headers, "\n") {
			key, value, _ := strings.Cut(line, " ")
			switch key {
			case "tree":
				c.tree = value
			case "parent":
				c.parents = append(c.parents, value)
			case "author":
				c.author = value
			case "encoding":
				c.encoding = value
			}
		}
		commits = append(commits, c)
	}
	return commits, nil
}

// commitTree writes c as a new commit with its author and author date; the
// committer is the current user, as in a rebase.
func commitTree(dir string, c rawCommit) (string, error) {
	lt, gt := strings.Index(c.author, " <"), strings.LastIndex(c.author, "> ")
	if lt < 0 || gt < lt {
		return "", fmt.Errorf("malformed author of %s: %q", c.hash, c.author)
	}
	env := []string{
		"GIT_AUTHOR_NAME=" + c.author[:lt],
		"GIT_AUTHOR_EMAIL=" + c.author[lt+2:gt],
		"GIT_AUTHOR_DATE=@" + c.author[gt+2:],
	}
	if c.encoding != "" {
		env = append(env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=i18n.commitEncoding", "GIT_CONFIG_VALUE_0="+c.encoding)
	}
	args := []string{"commit-tree", c.tree}
	for _, p := range c.parents {
		args = append(args, "-p", p)
	}
	out, err := gitInput(dir, c.message, env, args...)
	return strings.TrimSpace(string(out)), err
}

func revParse(dir, rev string) (string, error) {
	out, err := git(dir, "rev-parse", "--verify", "-q", rev)
	if err != nil {
		return "", fmt.Errorf("unknown revision %s", rev)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitPath resolves a path inside the git directory.
func gitPath(dir, name string) (string, error) {
	out, err := git(dir, "rev-parse", "--git-path", name)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	return path, nil
}

// gitInput runs git with stdin and extra environment variables.
func gitInput(dir, stdin string, env []string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return out, fmt.Errorf("git %s: %s", args[0], msg)
		}
		return out, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}
