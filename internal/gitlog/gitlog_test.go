package gitlog

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	// Shaped like `git log -M --numstat -z` output.
	out := strings.Join([]string{
		"\x1eaaa\x1fAlice\x1falice@example.com\x1f2023-11-15T06:13:20+08:00\x1fsubject",
		"\n3\t1\tmain.go",
		"-\t-\tlogo.png",
		"2\t0\t", "old dir/a.go", "new dir/a.go",
		"10\t0\tdocs/read me.md",
		"\x1ebbb\x1fBob\x1fbob@example.com\x1f2023-11-15T22:13:20Z\x1fsubject",
		"\x1eccc\x1fAlice\x1falice@example.com\x1f2023-11-16T22:13:20Z\x1fsubject",
		"\n1\t1\tmain.go",
		"",
	}, "\x00")

	commits, err := Parse(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 3 {
		t.Fatalf("got %d commits, want 3", len(commits))
	}
	c := commits[0]
	if c.Hash != "aaa" || c.Subject != "subject" || c.Author != "Alice" || c.Email != "alice@example.com" || c.Time.Unix() != 1700000000 {
		t.Errorf("unexpected header: %+v", c)
	}
	// The author's time zone is kept for the punch card.
	if _, off := c.Time.Zone(); c.Time.Hour() != 6 || off != 8*3600 {
		t.Errorf("author clock = %v, want 06:13 +08:00", c.Time)
	}
	want := []FileChange{
		{Path: "main.go", Added: 3, Deleted: 1},
		{Path: "logo.png", Added: -1, Deleted: -1},
		{Path: "new dir/a.go", OldPath: "old dir/a.go", Added: 2},
		{Path: "docs/read me.md", Added: 10},
	}
	if len(c.Files) != len(want) {
		t.Fatalf("got files %+v, want %+v", c.Files, want)
	}
	for i := range want {
		if c.Files[i] != want[i] {
			t.Errorf("file %d: got %+v, want %+v", i, c.Files[i], want[i])
		}
	}
	if len(commits[1].Files) != 0 || len(commits[2].Files) != 1 {
		t.Errorf("unexpected file counts: %d, %d", len(commits[1].Files), len(commits[2].Files))
	}
}

func TestParseMalformed(t *testing.T) {
	for _, in := range []string{
		"1\t1\tfile.go\x00",
		"\x1eaaa\x1fAlice\x00",
		"\x1eaaa\x1fA\x1fa@x\x1fnot-a-time\x1fsubject\x00",
		"\x1eaaa\x1fA\x1fa@x\x1f2023-11-15T06:13:20+08:00\x1fsubject\x00\nbad entry\x00",
		"\x1eaaa\x1fA\x1fa@x\x1f2023-11-15T06:13:20+08:00\x1fsubject\x00\n1\t0\t\x00old\x00",
	} {
		if _, err := Parse(strings.NewReader(in)); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", in)
		}
	}
}

func TestReadRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Alice", "GIT_AUTHOR_EMAIL=alice@example.com",
			"GIT_COMMITTER_NAME=Alice", "GIT_COMMITTER_EMAIL=alice@example.com",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")

	commits, err := Read(dir, Options{})
	if err != nil || len(commits) != 0 {
		t.Fatalf("empty repo: got %v, %v", commits, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "first")

	commits, err = Read(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 || commits[0].Author != "Alice" || len(commits[0].Files) != 1 || commits[0].Files[0] != (FileChange{Path: "a.txt", Added: 2}) {
		t.Fatalf("unexpected commits: %+v", commits)
	}
	// .mailmap rewrites the author identity.
	mailmap := "Alice Liddell <alice@wonderland.org> <alice@example.com>\n"
	if err := os.WriteFile(filepath.Join(dir, ".mailmap"), []byte(mailmap), 0o644); err != nil {
		t.Fatal(err)
	}
	commits, err = Read(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if c := commits[0]; c.Author != "Alice Liddell" || c.Email != "alice@wonderland.org" {
		t.Errorf("mailmap not applied: %s <%s>", c.Author, c.Email)
	}

	// Renames are detected.
	if err := os.Mkdir(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	run("mv", "a.txt", "docs/b.txt")
	run("commit", "-q", "-m", "move")
	commits, err = Read(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if f := commits[0].Files; len(f) != 1 || f[0] != (FileChange{Path: "docs/b.txt", OldPath: "a.txt"}) || commits[0].Subject != "move" {
		t.Errorf("rename not detected: %+v", commits[0])
	}
	if f, err := CommitFiles(dir, commits[0].Hash); err != nil || len(f) != 1 || f[0] != (FileChange{Path: "docs/b.txt", OldPath: "a.txt"}) {
		t.Errorf("CommitFiles = %+v, %v", f, err)
	}
	tracked, err := Tracked(dir)
	if err != nil || len(tracked) != 1 || tracked[0] != "docs/b.txt" {
		t.Errorf("Tracked = %v, %v", tracked, err)
	}

	run("checkout", "-q", "-b", "side")
	run("commit", "-q", "--allow-empty", "-m", "side work")
	run("checkout", "-q", "main")
	run("commit", "-q", "--allow-empty", "-m", "main work")
	// A stash must not count as work, even with All.
	if err := os.WriteFile(filepath.Join(dir, "docs/b.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("stash", "-q")
	commits, err = Read(dir, Options{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 4 {
		t.Errorf("All read %d commits, want 4 without the stash", len(commits))
	}

	head, err := Graph(dir, false, 100)
	if err != nil {
		t.Fatal(err)
	}
	every, err := Graph(dir, true, 100)
	if err != nil {
		t.Fatal(err)
	}
	// main and side diverged: both tips, a joining lane, then shared history.
	if len(head) != 3 || len(every) != 5 || every[1].Commit.Subject != "side work" || every[2].Commit != nil ||
		fmt.Sprint(head[0].Commit.Refs) != fmt.Sprint([]Ref{{"HEAD -> main", RefHead}}) {
		t.Errorf("graph: head %+v, all %+v", head, every)
	}
	if !strings.Contains(every[2].Graph, "\x1b[") {
		t.Errorf("branch lane not colored: %q", every[2].Graph)
	}
	if limited, _ := Graph(dir, true, 1); len(limited) != 1 {
		t.Errorf("limit ignored: %d lines", len(limited))
	}

	if b := Branch(dir); b != "main" {
		t.Errorf("Branch = %q, want main", b)
	}
	if _, err := TopLevel(t.TempDir()); err == nil {
		t.Error("TopLevel outside a repository succeeded")
	}

	// A merge lists the files it brings in from its second parent.
	run("checkout", "-q", "side")
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "c.txt")
	run("commit", "-q", "-m", "add c")
	run("checkout", "-q", "main")
	run("merge", "-q", "--no-edit", "side")
	if f, err := CommitFiles(dir, "HEAD"); err != nil || len(f) != 1 || f[0] != (FileChange{Path: "c.txt", Added: 1}) {
		t.Errorf("merge CommitFiles = %+v, %v", f, err)
	}
}

func TestParseGraph(t *testing.T) {
	out := "*   \x1ea1b2c3d\x1fa1b2c3d4e5\x1fHEAD -> refs/heads/main, refs/remotes/origin/main, tag: refs/tags/v1.0, refs/heads/feat/x\x1fMerge branch 'feat'\x1fBob\x1fbob@x\x1f1700000000\n" +
		"|\\  \n" +
		"| * \x1e9f8e7d6\x1f9f8e7d6aa\x1fHEAD, refs/stash\x1fAdd parser\x1fBob\x1fbob@x\x1f1699990000\n" +
		"|/  \n" +
		"* \x1e68b0c4d\x1f68b0c4dbb\x1f\x1fFix\x1fAlice\x1falice@x\x1f1699980000\n"
	lines, err := ParseGraph(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 5 {
		t.Fatalf("got %d lines", len(lines))
	}
	if l := lines[0]; l.Graph != "*" || l.Commit == nil || l.Commit.Author != "Bob" || l.Commit.FullHash != "a1b2c3d4e5" {
		t.Errorf("line 0 = %q %+v", l.Graph, l.Commit)
	}
	wantRefs := []Ref{{"HEAD -> main", RefHead}, {"origin/main", RefRemote}, {"v1.0", RefTag}, {"feat/x", RefBranch}}
	if got := lines[0].Commit.Refs; fmt.Sprint(got) != fmt.Sprint(wantRefs) {
		t.Errorf("refs = %v, want %v", got, wantRefs)
	}
	if got := lines[2].Commit.Refs; fmt.Sprint(got) != fmt.Sprint([]Ref{{"HEAD", RefHead}, {"stash", RefOther}}) {
		t.Errorf("detached refs = %v", got)
	}
	if l := lines[1]; l.Graph != "|\\" || l.Commit != nil {
		t.Errorf("line 1 = %q %+v", l.Graph, l.Commit)
	}
	if l := lines[2]; l.Graph != "| *" || l.Commit.Subject != "Add parser" {
		t.Errorf("line 2 = %q %+v", l.Graph, l.Commit)
	}
	if l := lines[4]; l.Commit.Refs != nil || l.Commit.Time.Unix() != 1699980000 {
		t.Errorf("line 4 = %+v", l.Commit)
	}
	if _, err := ParseGraph("* \x1ebad\n"); err == nil {
		t.Error("malformed line accepted")
	}
}
