package stats

import (
	"testing"
	"time"

	"github.com/ZhiWei-Ou/git-inktrace/internal/gitlog"
)

func at(y int, m time.Month, d, h int) time.Time {
	return time.Date(y, m, d, h, 0, 0, 0, time.Local)
}

func fc(path string, added, deleted int) gitlog.FileChange {
	return gitlog.FileChange{Path: path, Added: added, Deleted: deleted}
}

func sample() *Data {
	return New([]gitlog.Commit{
		{Author: "Bob", Email: "bob@example.com", Time: at(2026, 3, 11, 9),
			Files: []gitlog.FileChange{fc("a.go", 5, 1), fc("img.png", -1, -1)}},
		{Author: "Alice", Email: "Alice@Example.com", Time: at(2026, 3, 10, 18),
			Files: []gitlog.FileChange{fc("a.go", 2, 2)}},
		{Author: "alice", Email: "alice@example.com", Time: at(2026, 3, 10, 9),
			Files: []gitlog.FileChange{fc("b.go", 10, 0), fc("img.png", -1, -1)}},
		{Author: "Bob", Email: "bob@example.com", Time: at(2026, 3, 10, 8),
			Files: []gitlog.FileChange{fc("a.go", 1, 0)}},
		{Author: "Alice", Email: "alice@example.com", Time: at(2026, 3, 1, 12),
			Files: []gitlog.FileChange{fc("a.go", 1, 0)}},
	})
}

func TestAuthors(t *testing.T) {
	d := sample()
	if len(d.Authors) != 2 {
		t.Fatalf("got %d authors, want 2: %+v", len(d.Authors), d.Authors)
	}
	// Emails merge case-insensitively; the newest name wins.
	if a := d.Authors[0]; a.Name != "Alice" || a.Commits != 3 {
		t.Errorf("top author = %+v", a)
	}
	if a := d.Authors[1]; a.Name != "Bob" || a.Commits != 2 {
		t.Errorf("second author = %+v", a)
	}
	if !d.Earliest().Equal(at(2026, 3, 1, 12)) || !d.Latest().Equal(at(2026, 3, 11, 9)) {
		t.Errorf("span = %v .. %v", d.Earliest(), d.Latest())
	}
}

func TestAuthorIdentities(t *testing.T) {
	d := New([]gitlog.Commit{
		// Out of time order on purpose: the newest name must still win.
		{Author: "Bob Old", Email: "bob@example.com", Time: at(2026, 1, 1, 0)},
		{Author: "Robert", Email: "BOB@example.com", Time: at(2026, 6, 1, 0)},
		{Author: "Bob Mid", Email: "bob@example.com", Time: at(2026, 3, 1, 0)},
		// Same name under another email joins the same author, and that
		// email pulls in a further name (transitive merge).
		{Author: "Robert", Email: "robert@work.com", Time: at(2026, 2, 1, 0)},
		{Author: "R. Smith", Email: "robert@work.com", Time: at(2026, 2, 2, 0)},
		{Author: "Carol", Email: "carol@example.com", Time: at(2026, 7, 1, 0)},
	})
	if len(d.Authors) != 2 {
		t.Fatalf("got %d authors, want 2: %+v", len(d.Authors), d.Authors)
	}
	if a := d.Authors[0]; a.Name != "Robert" || a.Email != "BOB@example.com" || a.Commits != 5 {
		t.Errorf("merged author = %+v", a)
	}
	if a := d.Authors[1]; a.Name != "Carol" || a.Commits != 1 {
		t.Errorf("second author = %+v", a)
	}
}

func TestCalendar(t *testing.T) {
	d := sample()
	// 2026-03-12 is a Thursday.
	cal := d.Calendar(at(2026, 3, 12, 15), 2, AllAuthors)
	if !cal.Start.Equal(at(2026, 3, 1, 0)) || cal.Start.Weekday() != time.Sunday {
		t.Fatalf("start = %v", cal.Start)
	}
	if cal.Total != 5 || cal.Max != 3 {
		t.Errorf("total = %d, max = %d", cal.Total, cal.Max)
	}
	tue := cal.Weeks[1][time.Tuesday]
	if !tue.Date.Equal(at(2026, 3, 10, 0)) || tue.Total != 3 || tue.Top != 0 || tue.ByAuthor[1] != 1 {
		t.Errorf("Mar 10 = %+v", tue)
	}
	if wed := cal.Weeks[1][time.Wednesday]; wed.Total != 1 || wed.Top != 1 {
		t.Errorf("Mar 11 = %+v", wed)
	}
	if !cal.Weeks[1][time.Friday].Future || cal.Weeks[1][time.Thursday].Future {
		t.Error("future flags wrong")
	}
	for total, want := range []int{0, 2, 3, 4} {
		if got := cal.Level(total); got != want {
			t.Errorf("Level(%d) = %d, want %d", total, got, want)
		}
	}

	bob := d.Calendar(at(2026, 3, 12, 0), 2, 1)
	if bob.Total != 2 || bob.Weeks[1][time.Tuesday].Total != 1 || bob.Weeks[0][time.Sunday].Total != 0 {
		t.Errorf("bob calendar = %+v", bob)
	}

	// Commits before the first week are excluded.
	if one := d.Calendar(at(2026, 3, 12, 0), 1, AllAuthors); one.Total != 4 {
		t.Errorf("one week total = %d, want 4", one.Total)
	}
}

func TestAuthorCommits(t *testing.T) {
	d := sample()
	got := d.AuthorCommits(at(2026, 3, 10, 0), at(2026, 3, 31, 0))
	if len(got) != 2 || got[0] != 2 || got[1] != 2 {
		t.Errorf("AuthorCommits = %v, want [2 2]", got)
	}
}

func TestFileRank(t *testing.T) {
	d := sample()
	rank := d.FileRank(at(2026, 3, 1, 0), at(2026, 3, 31, 0), AllAuthors, 0)
	if len(rank) != 3 {
		t.Fatalf("got %d files: %+v", len(rank), rank)
	}
	a := rank[0]
	if a.Path != "a.go" || a.Commits != 4 || a.Added != 9 || a.Deleted != 3 || a.TopAuthor != 0 {
		t.Errorf("first = %+v", a)
	}
	if rank[1].Path != "img.png" || !rank[1].Binary || rank[2].Path != "b.go" {
		t.Errorf("order = %s, %s", rank[1].Path, rank[2].Path)
	}

	ranged := d.FileRank(at(2026, 3, 10, 0), at(2026, 3, 10, 0), 1, 0)
	if len(ranged) != 1 || ranged[0].Path != "a.go" || ranged[0].Commits != 1 {
		t.Errorf("ranged = %+v", ranged)
	}
	if limited := d.FileRank(at(2026, 1, 1, 0), at(2026, 12, 31, 0), AllAuthors, 1); len(limited) != 1 {
		t.Errorf("limit ignored: %d", len(limited))
	}
}
