package gitlog

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	out := strings.Join([]string{
		"\x1eaaa\x1fAlice\x1falice@example.com\x1f1700000000",
		"",
		"3\t1\tmain.go",
		"-\t-\tlogo.png",
		"10\t0\tdocs/read me.md",
		"\x1ebbb\x1fBob\x1fbob@example.com\x1f1700086400",
		"\x1eccc\x1fAlice\x1falice@example.com\x1f1700172800",
		"",
		"1\t1\tmain.go",
		"",
	}, "\n")

	commits, err := Parse(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 3 {
		t.Fatalf("got %d commits, want 3", len(commits))
	}
	c := commits[0]
	if c.Hash != "aaa" || c.Author != "Alice" || c.Email != "alice@example.com" || c.Time.Unix() != 1700000000 {
		t.Errorf("unexpected header: %+v", c)
	}
	want := []FileChange{{"main.go", 3, 1}, {"logo.png", -1, -1}, {"docs/read me.md", 10, 0}}
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
		"1\t1\tfile.go\n",
		"\x1eaaa\x1fAlice\n",
		"\x1eaaa\x1fA\x1fa@x\x1fnot-a-time\n",
		"\x1eaaa\x1fA\x1fa@x\x1f1\nbad line\n",
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
	if len(commits) != 1 || commits[0].Author != "Alice" || len(commits[0].Files) != 1 || commits[0].Files[0] != (FileChange{"a.txt", 2, 0}) {
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

	if b := Branch(dir); b != "main" {
		t.Errorf("Branch = %q, want main", b)
	}
	if _, err := TopLevel(t.TempDir()); err == nil {
		t.Error("TopLevel outside a repository succeeded")
	}
}
