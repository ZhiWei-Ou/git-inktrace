package gitlog

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// GraphLine is one line of `git log --graph` output.
type GraphLine struct {
	Graph  string       // the drawing prefix, e.g. "| *" or "|/", with git's lane colors as ANSI codes
	Commit *GraphCommit // nil for lines that only connect branches
}

// RefKind classifies a ref pointing at a commit.
type RefKind int

const (
	RefHead RefKind = iota // HEAD, attached or detached
	RefBranch
	RefRemote
	RefTag
	RefOther // e.g. refs/stash
)

// Ref is a decoration on a graph commit.
type Ref struct {
	Name string // short name, e.g. "HEAD -> main", "origin/main", "v1.0"
	Kind RefKind
}

// GraphCommit is a commit shown in the graph.
type GraphCommit struct {
	Hash    string // abbreviated
	Refs    []Ref
	Subject string
	Author  string
	Email   string
	Time    time.Time
}

// Graph returns the commit graph drawn by git, newest first, covering at most
// limit commits. With all set it includes every branch, remote branch and tag
// instead of only HEAD. Lanes keep git's colors (log.graphColors), while the
// commit fields stay plain. Author names and emails have .mailmap applied.
func Graph(dir string, all bool, limit int) ([]GraphLine, error) {
	args := []string{
		"log", "--graph", "--color=always", "--decorate=full", "--max-count=" + strconv.Itoa(limit),
		"--format=" + recordSep + strings.Join([]string{"%h", "%D", "%s", "%aN", "%aE", "%at"}, fieldSep),
	}
	if all {
		args = append(args, "HEAD", "--branches", "--remotes", "--tags")
	}
	out, err := git(dir, args...)
	if err != nil {
		if strings.Contains(err.Error(), "does not have any commits") {
			return nil, nil
		}
		return nil, err
	}
	return ParseGraph(string(out))
}

// ParseGraph decodes the output of the git log command built by Graph.
func ParseGraph(out string) ([]GraphLine, error) {
	var lines []GraphLine
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		prefix, rest, isCommit := strings.Cut(line, recordSep)
		gl := GraphLine{Graph: strings.TrimRight(prefix, " ")}
		if isCommit {
			f := strings.Split(rest, fieldSep)
			if len(f) != 6 {
				return nil, fmt.Errorf("malformed graph line: %q", line)
			}
			ts, err := strconv.ParseInt(f[5], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("malformed commit time %q: %w", f[5], err)
			}
			gl.Commit = &GraphCommit{Hash: f[0], Refs: parseRefs(f[1]), Subject: f[2], Author: f[3], Email: f[4], Time: time.Unix(ts, 0)}
		}
		lines = append(lines, gl)
	}
	return lines, nil
}

// parseRefs decodes %D output of --decorate=full, such as
// "HEAD -> refs/heads/main, refs/remotes/origin/main, tag: refs/tags/v1.0".
func parseRefs(s string) []Ref {
	if s == "" {
		return nil
	}
	var refs []Ref
	for _, r := range strings.Split(s, ", ") {
		switch {
		case strings.HasPrefix(r, "HEAD -> "):
			refs = append(refs, Ref{Name: "HEAD -> " + strings.TrimPrefix(r, "HEAD -> refs/heads/"), Kind: RefHead})
		case r == "HEAD":
			refs = append(refs, Ref{Name: "HEAD", Kind: RefHead})
		case strings.HasPrefix(r, "tag: "):
			refs = append(refs, Ref{Name: strings.TrimPrefix(strings.TrimPrefix(r, "tag: "), "refs/tags/"), Kind: RefTag})
		case strings.HasPrefix(r, "refs/heads/"):
			refs = append(refs, Ref{Name: strings.TrimPrefix(r, "refs/heads/"), Kind: RefBranch})
		case strings.HasPrefix(r, "refs/remotes/"):
			refs = append(refs, Ref{Name: strings.TrimPrefix(r, "refs/remotes/"), Kind: RefRemote})
		default:
			refs = append(refs, Ref{Name: strings.TrimPrefix(r, "refs/"), Kind: RefOther})
		}
	}
	return refs
}
