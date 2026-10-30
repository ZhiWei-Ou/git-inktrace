// Package gitlog reads commit history from a git repository.
package gitlog

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// FileChange is one file touched by a commit.
type FileChange struct {
	Path    string
	OldPath string // previous path when the commit renamed the file
	Added   int    // -1 for binary files
	Deleted int    // -1 for binary files
}

// Commit is a non-merge commit with its file changes.
type Commit struct {
	Hash    string
	Subject string
	Author  string
	Email   string
	Time    time.Time // author date, in the author's time zone
	Files   []FileChange
}

const (
	recordSep = "\x1e"
	fieldSep  = "\x1f"
)

// commitFormat is the header Parse expects before each commit's numstat.
var commitFormat = "--format=" + recordSep + strings.Join([]string{"%H", "%aN", "%aE", "%aI", "%s"}, fieldSep)

// TopLevel returns the root directory of the work tree containing dir.
func TopLevel(dir string) (string, error) {
	out, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not a git repository: %s", dir)
	}
	return strings.TrimSpace(string(out)), nil
}

// Branch returns the current branch name, or the short HEAD hash when detached.
func Branch(dir string) string {
	if out, err := git(dir, "symbolic-ref", "--short", "-q", "HEAD"); err == nil {
		return strings.TrimSpace(string(out))
	}
	if out, err := git(dir, "rev-parse", "--short", "HEAD"); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

// allRefs selects HEAD, every branch, remote branch and tag. Unlike --all it
// leaves out refs/stash, whose index commits are not real work.
var allRefs = []string{"HEAD", "--branches", "--remotes", "--tags"}

// Options selects which history to read.
type Options struct {
	All bool // read every branch, remote branch and tag instead of HEAD
}

// Read returns the non-merge commits reachable from HEAD (or all refs),
// newest first, with their file changes. Author names and emails have
// .mailmap applied, and renames are detected. A repository without commits
// yields an empty slice.
func Read(dir string, opts Options) ([]Commit, error) {
	commits, err := ReadHeaders(dir, opts)
	if err != nil {
		return nil, err
	}
	files, err := ReadFiles(dir, commits, runtime.NumCPU(), nil)
	if err != nil {
		return nil, err
	}
	for i := range commits {
		commits[i].Files = files[i]
	}
	return commits, nil
}

// ReadHeaders returns the commits Read would, without their file changes.
// Comparing every commit with its parent is nearly all the work of Read, so
// this is many times faster on a large history.
func ReadHeaders(dir string, opts Options) ([]Commit, error) {
	args := []string{"log", "--no-merges", "-z", commitFormat}
	if opts.All {
		args = append(args, allRefs...)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	commits, parseErr := Parse(stdout)
	// Drain the pipe so git can exit if parsing stopped early.
	_, _ = io.Copy(io.Discard, stdout)
	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		// An unborn HEAD has no history to show.
		if strings.Contains(msg, "does not have any commits") {
			return nil, nil
		}
		return nil, fmt.Errorf("git log: %s", msg)
	}
	return commits, parseErr
}

// ReadFiles returns the file changes of each commit, compared with its
// parent and with renames detected, indexed like commits. Up to workers git
// processes share the commits; done, when not nil, counts the commits read
// so far.
func ReadFiles(dir string, commits []Commit, workers int, done *atomic.Int64) ([][]FileChange, error) {
	files := make([][]FileChange, len(commits))
	index := make(map[string]int, len(commits))
	for i, c := range commits {
		index[c.Hash] = i
	}
	workers = max(min(workers, len(commits)), 1)
	errs := make(chan error, workers)
	for w := 0; w < workers; w++ {
		// Every workers-th commit, so that costly stretches of history spread
		// over all processes.
		var hashes strings.Builder
		for i := w; i < len(commits); i += workers {
			hashes.WriteString(commits[i].Hash + "\n")
		}
		go func() { errs <- diffTree(dir, hashes.String(), index, files, done) }()
	}
	var err error
	for w := 0; w < workers; w++ {
		if e := <-errs; e != nil && err == nil {
			err = e
		}
	}
	return files, err
}

// diffTree reads the numstat of the commits listed in hashes into files at
// their index. Its output is each commit hash followed by numstat tokens; a
// rename is a token with an empty path followed by the old and new paths.
func diffTree(dir, hashes string, index map[string]int, files [][]FileChange, done *atomic.Int64) error {
	cmd := exec.Command("git", "diff-tree", "--stdin", "-r", "-M", "--numstat", "-z", "--root")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(hashes)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	parseErr := func() error {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		sc.Split(splitNUL)
		current := -1
		for sc.Scan() {
			tok := sc.Text()
			if !strings.Contains(tok, "\t") {
				i, ok := index[strings.TrimSpace(tok)]
				if !ok {
					return fmt.Errorf("git diff-tree: unexpected %q", tok)
				}
				current = i
				if done != nil {
					done.Add(1)
				}
				continue
			}
			if current < 0 {
				return fmt.Errorf("git diff-tree: changes before a commit: %q", tok)
			}
			fc, err := parseNumstat(tok)
			if err != nil {
				return err
			}
			if fc.Path == "" {
				if !sc.Scan() {
					return fmt.Errorf("truncated rename after %q", tok)
				}
				fc.OldPath = sc.Text()
				if !sc.Scan() {
					return fmt.Errorf("truncated rename after %q", tok)
				}
				fc.Path = sc.Text()
			}
			files[current] = append(files[current], fc)
		}
		return sc.Err()
	}()
	_, _ = io.Copy(io.Discard, stdout)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git diff-tree: %s", strings.TrimSpace(stderr.String()))
	}
	return parseErr
}

// BranchCount returns the number of local and remote-tracking branches,
// leaving out symbolic refs such as origin/HEAD.
func BranchCount(dir string) (int, error) {
	out, err := git(dir, "for-each-ref", "--format=%(symref)", "refs/heads", "refs/remotes")
	if err != nil {
		return 0, err
	}
	n := 0
	for line := range strings.Lines(string(out)) {
		if strings.TrimSpace(line) == "" {
			n++
		}
	}
	return n, nil
}

// CommitFiles returns the files changed by one commit, with renames
// detected. A merge is compared with its first parent.
func CommitFiles(dir, hash string) ([]FileChange, error) {
	out, err := git(dir, "show", "--diff-merges=first-parent", "-M", "--numstat", "-z", commitFormat, hash, "--")
	if err != nil {
		return nil, err
	}
	commits, err := Parse(bytes.NewReader(out))
	if err != nil || len(commits) == 0 {
		return nil, err
	}
	return commits[0].Files, nil
}

// CommitBody returns the message of one commit without its subject, with
// trailing blank lines removed.
func CommitBody(dir, hash string) (string, error) {
	out, err := git(dir, "show", "-s", "--format=%b", hash, "--")
	return strings.TrimRight(string(out), "\n"), err
}

// FirstCommitTime returns the earliest author date among the root commits of
// HEAD, every branch, remote branch and tag: when the repository started,
// whichever branch is checked out. It is zero in a repository without commits.
func FirstCommitTime(dir string) (time.Time, error) {
	out, err := git(dir, append([]string{"log", "--max-parents=0", "--format=%aI"}, allRefs...)...)
	if err != nil {
		if strings.Contains(err.Error(), "does not have any commits") {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	var first time.Time
	for line := range strings.FieldsSeq(string(out)) {
		t, err := time.Parse(time.RFC3339, line)
		if err != nil {
			return time.Time{}, fmt.Errorf("malformed commit time %q: %w", line, err)
		}
		if first.IsZero() || t.Before(first) {
			first = t
		}
	}
	return first, nil
}

// Tracked returns the paths of the files in the HEAD tree.
func Tracked(dir string) ([]string, error) {
	out, err := git(dir, "ls-tree", "-r", "-z", "--name-only", "HEAD")
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00"), nil
}

// Parse decodes the NUL-separated output of the git log command built by Read.
// Each commit is a header token followed by numstat tokens; a rename is a
// numstat token with an empty path followed by the old and new path tokens.
func Parse(r io.Reader) ([]Commit, error) {
	var commits []Commit
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	sc.Split(splitNUL)
	next := func() (string, bool) {
		if !sc.Scan() {
			return "", false
		}
		return sc.Text(), true
	}
	for {
		tok, ok := next()
		if !ok {
			break
		}
		tok = strings.TrimLeft(tok, "\n")
		if tok == "" {
			continue
		}
		if strings.HasPrefix(tok, recordSep) {
			c, err := parseHeader(tok[len(recordSep):])
			if err != nil {
				return nil, err
			}
			commits = append(commits, c)
			continue
		}
		if len(commits) == 0 {
			return nil, fmt.Errorf("unexpected output before first commit: %q", tok)
		}
		fc, err := parseNumstat(tok)
		if err != nil {
			return nil, err
		}
		if fc.Path == "" {
			old, ok1 := next()
			path, ok2 := next()
			if !ok1 || !ok2 {
				return nil, fmt.Errorf("truncated rename after %q", tok)
			}
			fc.OldPath, fc.Path = old, path
		}
		last := &commits[len(commits)-1]
		last.Files = append(last.Files, fc)
	}
	return commits, sc.Err()
}

func splitNUL(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func parseHeader(s string) (Commit, error) {
	f := strings.Split(s, fieldSep)
	if len(f) != 5 {
		return Commit{}, fmt.Errorf("malformed commit header: %q", s)
	}
	ts, err := time.Parse(time.RFC3339, f[3])
	if err != nil {
		return Commit{}, fmt.Errorf("malformed commit time %q: %w", f[3], err)
	}
	return Commit{Hash: f[0], Author: f[1], Email: f[2], Time: ts, Subject: f[4]}, nil
}

// parseNumstat decodes "added\tdeleted\tpath"; path is empty for a rename.
func parseNumstat(line string) (FileChange, error) {
	f := strings.SplitN(line, "\t", 3)
	if len(f) != 3 {
		return FileChange{}, fmt.Errorf("malformed numstat entry: %q", line)
	}
	return FileChange{Path: f[2], Added: count(f[0]), Deleted: count(f[1])}, nil
}

func count(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1 // "-" marks a binary file
	}
	return n
}

func git(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out, fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(ee.Stderr)))
	}
	return out, err
}
