package stats

import (
	"fmt"
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
	}, nil)
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
	}, nil)
	if len(d.Authors) != 2 {
		t.Fatalf("got %d authors, want 2: %+v", len(d.Authors), d.Authors)
	}
	if a := d.Authors[0]; a.Name != "Robert" || a.Email != "BOB@example.com" || a.Commits != 5 {
		t.Errorf("merged author = %+v", a)
	}
	if a := d.Authors[1]; a.Name != "Carol" || a.Commits != 1 {
		t.Errorf("second author = %+v", a)
	}
	if d.AuthorOf("R. Smith", "") != 0 || d.AuthorOf("", "Carol@Example.com") != 1 || d.AuthorOf("Zed", "z@x") != -1 {
		t.Error("AuthorOf lookups wrong")
	}
}

func TestFilter(t *testing.T) {
	d := sample()
	for pattern, want := range map[string]string{"ALICE": "[0]", "example.com": "[0 1]", "bob@": "[1]", "zed": "[]"} {
		if got := fmt.Sprint(d.Match(pattern)); got != want {
			t.Errorf("Match(%q) = %s, want %s", pattern, got, want)
		}
	}
	if d.Filter(nil) != d {
		t.Error("no patterns did not keep the data")
	}
	bob := d.Filter([]string{"zed", "bob"})
	if len(bob.Authors) != 1 || bob.Authors[0].Name != "Bob" || bob.Len() != 2 {
		t.Errorf("filtered to %+v, %d commits", bob.Authors, bob.Len())
	}
	if bob.AuthorOf("Bob", "bob@example.com") != 0 || bob.AuthorOf("Alice", "alice@example.com") != -1 {
		t.Error("AuthorOf after filter wrong")
	}
	// Patterns join: both authors stay in rank order.
	if both := d.Filter([]string{"bob", "Alice@"}); len(both.Authors) != 2 || both.Authors[0].Name != "Alice" || both.Len() != 5 {
		t.Errorf("two patterns kept %+v", both.Authors)
	}
}

func TestCalendar(t *testing.T) {
	d := sample()
	// 2026-03-12 is a Thursday.
	cal := d.Calendar(at(2026, 3, 12, 15), 2)
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

	bob := d.Filter([]string{"bob"}).Calendar(at(2026, 3, 12, 0), 2)
	if bob.Total != 2 || bob.Weeks[1][time.Tuesday].Total != 1 || bob.Weeks[0][time.Sunday].Total != 0 {
		t.Errorf("bob calendar = %+v", bob)
	}

	// Commits before the first week are excluded.
	if one := d.Calendar(at(2026, 3, 12, 0), 1); one.Total != 4 {
		t.Errorf("one week total = %d, want 4", one.Total)
	}
}

func TestPunchCard(t *testing.T) {
	d := sample()
	card := d.PunchCard(at(2026, 3, 1, 0), at(2026, 3, 31, 0))
	if card.Total != 5 || card.Max != 1 {
		t.Errorf("total = %d, max = %d", card.Total, card.Max)
	}
	// Mar 10 is a Tuesday and Mar 11 a Wednesday.
	if s := card.Slots[time.Tuesday][9]; s.Total != 1 || s.Top != 0 {
		t.Errorf("Tue 09 = %+v", s)
	}
	if s := card.Slots[time.Wednesday][9]; s.Total != 1 || s.Top != 1 {
		t.Errorf("Wed 09 = %+v", s)
	}
	if s := card.Slots[time.Monday][0]; s.Total != 0 || s.Top != -1 {
		t.Errorf("empty slot = %+v", s)
	}
	if alice := card.ByAuthor[0]; alice == nil || alice[time.Tuesday][18] != 1 || alice[time.Tuesday][9] != 1 || alice[time.Sunday][12] != 1 {
		t.Errorf("Alice's hours = %v", card.ByAuthor[0])
	}
	if _, off := at(2026, 3, 10, 9).Zone(); card.Zones[off] != 5 {
		t.Errorf("zones = %v", card.Zones)
	}

	if bob := d.Filter([]string{"bob"}).PunchCard(at(2026, 3, 1, 0), at(2026, 3, 31, 0)); bob.Total != 2 || bob.Slots[time.Tuesday][8].Top != 0 {
		t.Errorf("bob card = %+v", bob)
	}
	if early := d.PunchCard(at(2026, 3, 1, 0), at(2026, 3, 9, 0)); early.Total != 1 {
		t.Errorf("window total = %d, want 1", early.Total)
	}
}

func TestPunchCardUsesAuthorClock(t *testing.T) {
	tokyo := time.FixedZone("JST", 9*3600)
	// Saturday 23:30 in Tokyo, whatever the local zone is.
	when := time.Date(2026, 3, 14, 23, 30, 0, 0, tokyo)
	d := New([]gitlog.Commit{{Author: "Aki", Email: "aki@example.jp", Time: when}}, nil)
	card := d.PunchCard(when.AddDate(0, 0, -2), when.AddDate(0, 0, 2))
	if card.Slots[time.Saturday][23].Total != 1 {
		t.Errorf("commit not at Sat 23: %+v", card.Slots)
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
	rank := d.FileRank(at(2026, 3, 1, 0), at(2026, 3, 31, 0), 0)
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

	ranged := d.Filter([]string{"bob"}).FileRank(at(2026, 3, 10, 0), at(2026, 3, 10, 0), 0)
	if len(ranged) != 1 || ranged[0].Path != "a.go" || ranged[0].Commits != 1 {
		t.Errorf("ranged = %+v", ranged)
	}
	if limited := d.FileRank(at(2026, 1, 1, 0), at(2026, 12, 31, 0), 1); len(limited) != 1 {
		t.Errorf("limit ignored: %d", len(limited))
	}
}

func TestFileCommits(t *testing.T) {
	d := New([]gitlog.Commit{ // newest first
		{Hash: "c3", Subject: "edit", Author: "A", Email: "a@x", Time: at(2026, 3, 3, 0), Files: []gitlog.FileChange{fc("new.go", 2, 1)}},
		{Hash: "c2", Subject: "move", Author: "B", Email: "b@x", Time: at(2026, 3, 2, 0),
			Files: []gitlog.FileChange{{Path: "new.go", OldPath: "old.go"}, fc("other.go", 1, 0)}},
		{Hash: "c1", Subject: "start", Author: "A", Email: "a@x", Time: at(2026, 3, 1, 0), Files: []gitlog.FileChange{fc("old.go", 9, 0)}},
	}, nil)
	var got []string
	for _, c := range d.FileCommits("new.go", at(2026, 1, 1, 0), at(2026, 12, 31, 0)) {
		got = append(got, fmt.Sprintf("%s %s %d +%d -%d", c.Hash, c.Subject, c.Author, c.Added, c.Deleted))
	}
	// The history follows the rename back to the old path.
	if want := []string{"c3 edit 0 +2 -1", "c2 move 1 +0 -0", "c1 start 0 +9 -0"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if n := len(d.Filter([]string{"b@x"}).FileCommits("new.go", at(2026, 3, 2, 0), at(2026, 3, 3, 0))); n != 1 {
		t.Errorf("date and author filter: %d commits, want 1", n)
	}
}

func TestFileRankFollowsRenames(t *testing.T) {
	rename := func(from, to string, added int) gitlog.FileChange {
		return gitlog.FileChange{Path: to, OldPath: from, Added: added}
	}
	d := New([]gitlog.Commit{ // newest first
		{Author: "A", Email: "a@x", Time: at(2026, 3, 6, 0), Files: []gitlog.FileChange{fc("old/a.go", 5, 0)}},
		{Author: "A", Email: "a@x", Time: at(2026, 3, 5, 0), Files: []gitlog.FileChange{fc("new/a.go", 1, 1)}},
		{Author: "A", Email: "a@x", Time: at(2026, 3, 4, 0), Files: []gitlog.FileChange{rename("mid/a.go", "new/a.go", 1)}},
		{Author: "A", Email: "a@x", Time: at(2026, 3, 3, 0), Files: []gitlog.FileChange{rename("old/a.go", "mid/a.go", 0)}},
		{Author: "A", Email: "a@x", Time: at(2026, 3, 2, 0), Files: []gitlog.FileChange{fc("old/a.go", 3, 1)}},
		{Author: "A", Email: "a@x", Time: at(2026, 3, 1, 0), Files: []gitlog.FileChange{fc("old/a.go", 10, 0), fc("gone.go", 4, 0)}},
		{Author: "A", Email: "a@x", Time: at(2026, 2, 28, 0), Files: []gitlog.FileChange{fc("gone.go", 0, 4)}},
	}, []string{"new/a.go", "old/a.go"})

	rank := d.FileRank(at(2026, 1, 1, 0), at(2026, 12, 31, 0), 0)
	if len(rank) != 3 {
		t.Fatalf("got %+v", rank)
	}
	// The whole rename chain counts toward the latest path.
	if r := rank[0]; r.Path != "new/a.go" || r.Commits != 5 || r.Added != 15 || r.Deleted != 2 || r.Gone {
		t.Errorf("renamed file = %+v", r)
	}
	// A new file reusing the old path after the rename stays separate.
	for _, r := range rank[1:] {
		switch r.Path {
		case "old/a.go":
			if r.Commits != 1 || r.Gone {
				t.Errorf("reused path = %+v", r)
			}
		case "gone.go":
			if !r.Gone {
				t.Errorf("deleted file not marked: %+v", r)
			}
		default:
			t.Errorf("unexpected path %+v", r)
		}
	}
}
