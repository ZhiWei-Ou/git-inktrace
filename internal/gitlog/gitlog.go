// Package gitlog reads commit history from a git repository.
package gitlog

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// FileChange is one file touched by a commit.
type FileChange struct {
	Path    string
	Added   int // -1 for binary files
	Deleted int // -1 for binary files
}

// Commit is a non-merge commit with its file changes.
type Commit struct {
	Hash   string
	Author string
	Email  string
	Time   time.Time
	Files  []FileChange
}

const (
	recordSep = "\x1e"
	fieldSep  = "\x1f"
)

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

// Options selects which history to read.
type Options struct {
	All bool // read all refs instead of HEAD
}

// Read returns the non-merge commits reachable from HEAD (or all refs), newest first.
// A repository without commits yields an empty slice.
func Read(dir string, opts Options) ([]Commit, error) {
	args := []string{
		"-c", "core.quotePath=false",
		"log", "--no-merges", "--no-renames", "--numstat",
		"--format=" + recordSep + "%H" + fieldSep + "%aN" + fieldSep + "%aE" + fieldSep + "%at",
	}
	if opts.All {
		args = append(args, "--all")
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

// Parse decodes the output of the git log command built by Read.
func Parse(r io.Reader) ([]Commit, error) {
	var commits []Commit
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, recordSep) {
			c, err := parseHeader(line[len(recordSep):])
			if err != nil {
				return nil, err
			}
			commits = append(commits, c)
			continue
		}
		if len(commits) == 0 {
			return nil, fmt.Errorf("unexpected line before first commit: %q", line)
		}
		fc, err := parseNumstat(line)
		if err != nil {
			return nil, err
		}
		last := &commits[len(commits)-1]
		last.Files = append(last.Files, fc)
	}
	return commits, sc.Err()
}

func parseHeader(s string) (Commit, error) {
	f := strings.Split(s, fieldSep)
	if len(f) != 4 {
		return Commit{}, fmt.Errorf("malformed commit header: %q", s)
	}
	ts, err := strconv.ParseInt(f[3], 10, 64)
	if err != nil {
		return Commit{}, fmt.Errorf("malformed commit time %q: %w", f[3], err)
	}
	return Commit{Hash: f[0], Author: f[1], Email: f[2], Time: time.Unix(ts, 0)}, nil
}

func parseNumstat(line string) (FileChange, error) {
	f := strings.SplitN(line, "\t", 3)
	if len(f) != 3 {
		return FileChange{}, fmt.Errorf("malformed numstat line: %q", line)
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
