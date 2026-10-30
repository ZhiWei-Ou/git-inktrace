package gitlog

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// rewordRepo builds main: first, second, a side branch off second merged
// back, then last. It returns the repository and a git runner.
func rewordRepo(t *testing.T) (string, func(args ...string) string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_COMMITTER_NAME", "Bob")
	t.Setenv("GIT_COMMITTER_EMAIL", "bob@example.com")
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Alice", "GIT_AUTHOR_EMAIL=alice@example.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	commit := func(file, msg, date string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, file), []byte(msg+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", file)
		run("commit", "-q", "-m", msg, "--date", date)
	}
	run("init", "-q", "-b", "main")
	commit("a.txt", "first", "2026-01-01T10:00:00+09:00")
	commit("b.txt", "second", "2026-01-02T10:00:00+09:00")
	run("switch", "-q", "-c", "side")
	commit("c.txt", "side work", "2026-01-03T10:00:00+09:00")
	run("switch", "-q", "main")
	commit("d.txt", "main work", "2026-01-04T10:00:00+09:00")
	run("merge", "-q", "--no-edit", "side")
	commit("e.txt", "last", "2026-01-05T10:00:00+09:00")
	return dir, run
}

func TestReword(t *testing.T) {
	dir, run := rewordRepo(t)
	second := run("rev-parse", "HEAD~1^2~1") // via the side branch
	trees := run("log", "--format=%T %ad %an", "--date=raw", "HEAD")
	// Uncommitted work survives, since no tree changes.
	if err := os.WriteFile(filepath.Join(dir, "e.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	remotes, err := CheckReword(dir, second)
	if err != nil || len(remotes) != 0 {
		t.Fatalf("CheckReword = %v, %v", remotes, err)
	}
	reworded, err := Reword(dir, second, "Second, reworded  \n\nWith a body.\n\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := run("log", "-1", "--format=%B", reworded); got != "Second, reworded\n\nWith a body." {
		t.Errorf("message = %q", got)
	}
	if body, err := CommitBody(dir, reworded); body != "With a body." || err != nil {
		t.Errorf("CommitBody = %q, %v", body, err)
	}
	if got := run("log", "--format=%s", "HEAD"); got != "last\nMerge branch 'side'\nmain work\nside work\nSecond, reworded\nfirst" {
		t.Errorf("history:\n%s", got)
	}
	if got := run("log", "--format=%T %ad %an", "--date=raw", "HEAD"); got != trees {
		t.Errorf("trees, author dates or authors changed:\n%s\nwant\n%s", got, trees)
	}
	if got := run("rev-list", "--parents", "-1", "HEAD~1"); len(strings.Fields(got)) != 3 {
		t.Errorf("merge lost a parent: %s", got)
	}
	if got := run("symbolic-ref", "--short", "HEAD"); got != "main" {
		t.Errorf("HEAD left the branch: %s", got)
	}
	if got := run("status", "--porcelain"); got != "M e.txt" {
		t.Errorf("status = %q", got)
	}
	// The side branch keeps the original commits.
	if got := run("log", "-1", "--format=%s", "side~1"); got != "second" {
		t.Errorf("side branch rewritten: %s", got)
	}
}

func TestFirstCommitTime(t *testing.T) {
	dir, run := rewordRepo(t)
	// An older root on another branch counts, though HEAD does not reach it.
	run("switch", "-q", "--orphan", "old")
	run("commit", "-q", "--allow-empty", "-m", "old root", "--date", "2025-06-01T08:00:00-05:00")
	run("switch", "-q", "main")
	first, err := FirstCommitTime(dir)
	if err != nil || first.Unix() != 1748782800 {
		t.Errorf("FirstCommitTime = %v, %v", first, err)
	}
}

func TestRewordHeadAndRoot(t *testing.T) {
	dir, run := rewordRepo(t)
	if _, err := Reword(dir, "HEAD", "tip\n"); err != nil {
		t.Fatal(err)
	}
	root := run("rev-list", "--max-parents=0", "HEAD")
	if _, err := Reword(dir, root, "root\n"); err != nil {
		t.Fatal(err)
	}
	if got := run("log", "--format=%s", "--first-parent", "HEAD"); got != "tip\nMerge branch 'side'\nmain work\nsecond\nroot" {
		t.Errorf("history:\n%s", got)
	}
}

func TestRewordRefuses(t *testing.T) {
	dir, run := rewordRepo(t)
	head := run("rev-parse", "HEAD")
	if _, err := Reword(dir, "HEAD", "last\n"); !errors.Is(err, ErrMessageUnchanged) {
		t.Errorf("unchanged: %v", err)
	}
	if _, err := Reword(dir, "HEAD", "\n  \n"); !errors.Is(err, ErrMessageEmpty) {
		t.Errorf("empty: %v", err)
	}
	if run("rev-parse", "HEAD") != head {
		t.Error("HEAD moved")
	}

	run("switch", "-q", "-c", "topic", "HEAD~3")
	run("commit", "-q", "--allow-empty", "-m", "topic only")
	if _, err := CheckReword(dir, "main"); err == nil {
		t.Error("CheckReword accepted a commit off the current branch")
	}

	// A remote-tracking branch holding the commit is reported.
	run("update-ref", "refs/remotes/origin/main", "main~3")
	remotes, err := CheckReword(dir, "topic~1")
	if err != nil || strings.Join(remotes, " ") != "origin/main" {
		t.Errorf("remotes = %v, %v", remotes, err)
	}
}

func TestEditMessageCommand(t *testing.T) {
	dir, _ := rewordRepo(t)
	t.Setenv("GIT_EDITOR", `sh -c 'printf "edited\n" >> "$1"' -`)
	cmd, path, err := EditMessageCommand(dir, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("editor: %v\n%s", err, out)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "last\nedited\n" {
		t.Errorf("message file = %q, %v", got, err)
	}
}
