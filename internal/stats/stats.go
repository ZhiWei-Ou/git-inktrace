// Package stats aggregates commit history into calendars, punch cards and
// file rankings.
package stats

import (
	"sort"
	"strings"
	"time"

	"github.com/ZhiWei-Ou/git-inktrace/internal/gitlog"
)

// AllAuthors selects every author in Calendar and FileRank.
const AllAuthors = -1

// Author is one contributor, possibly committing under several names or
// emails, ordered by commit count.
type Author struct {
	Name    string // name of the newest commit
	Email   string // email of the newest commit
	Commits int
	latest  time.Time
}

type commit struct {
	hash    string
	subject string
	author  int
	day     time.Time // local midnight
	time    time.Time
	clock   time.Time           // author date in the author's time zone
	files   []gitlog.FileChange // paths resolved to the file's latest name
}

// Data holds the aggregated history of a repository.
type Data struct {
	Authors    []Author // sorted by commits, descending
	commits    []commit // newest first
	tracked    map[string]bool
	identities map[string]int // identity key -> author index
}

// New indexes commits by author. Commits sharing an email (case-insensitive)
// or a name belong to the same author, like `git shortlog` grouping, so
// identities unified by .mailmap (already applied by gitlog) merge as well.
// Each author shows the name and email of their newest commit.
//
// File changes follow renames, so a file's whole history counts toward its
// latest path. tracked lists the paths that still exist; files outside it
// are reported as gone. A nil tracked marks nothing as gone.
func New(commits []gitlog.Commit, tracked []string) *Data {
	ids := groupIdentities(commits)
	files := resolveRenames(commits)
	index := map[int]int{} // group -> author
	var authors []Author
	for i, c := range commits {
		id, ok := index[ids[i]]
		if !ok {
			id = len(authors)
			index[ids[i]] = id
			authors = append(authors, Author{})
		}
		a := &authors[id]
		a.Commits++
		if a.Commits == 1 || c.Time.After(a.latest) {
			a.Name, a.Email, a.latest = c.Author, c.Email, c.Time
		}
		ids[i] = id
	}

	order := make([]int, len(authors))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		x, y := authors[order[a]], authors[order[b]]
		if x.Commits != y.Commits {
			return x.Commits > y.Commits
		}
		return strings.ToLower(x.Name) < strings.ToLower(y.Name)
	})
	rank := make([]int, len(authors))
	d := &Data{Authors: make([]Author, len(authors)), commits: make([]commit, len(commits))}
	for r, id := range order {
		rank[id] = r
		d.Authors[r] = authors[id]
	}
	if tracked != nil {
		d.tracked = make(map[string]bool, len(tracked))
		for _, p := range tracked {
			d.tracked[p] = true
		}
	}
	d.identities = map[string]int{}
	for i, c := range commits {
		local := c.Time.Local()
		d.commits[i] = commit{
			hash: c.Hash, subject: c.Subject, author: rank[ids[i]],
			day: midnight(local), time: local, clock: c.Time, files: files[i],
		}
		for _, k := range identityKeys(c.Author, c.Email) {
			d.identities[k] = rank[ids[i]]
		}
	}
	return d
}

// AuthorOf returns the index of the author committing as name and email,
// or -1 when no counted commit used that identity.
func (d *Data) AuthorOf(name, email string) int {
	for _, k := range identityKeys(name, email) {
		if id, ok := d.identities[k]; ok {
			return id
		}
	}
	return -1
}

// Len returns the number of commits.
func (d *Data) Len() int { return len(d.commits) }

// Latest returns the time of the newest commit, or zero when there is none.
func (d *Data) Latest() time.Time {
	var t time.Time
	for _, c := range d.commits {
		if c.time.After(t) {
			t = c.time
		}
	}
	return t
}

// Earliest returns the time of the oldest commit, or zero when there is none.
func (d *Data) Earliest() time.Time {
	var t time.Time
	for _, c := range d.commits {
		if t.IsZero() || c.time.Before(t) {
			t = c.time
		}
	}
	return t
}

// Day is one calendar cell.
type Day struct {
	Date     time.Time
	Total    int
	ByAuthor map[int]int // author index -> commits
	Top      int         // author with most commits that day, -1 when empty
	Future   bool        // after the calendar end date
}

// Calendar is a GitHub-style grid of weeks (columns) by weekdays (rows, Sunday first).
type Calendar struct {
	Start, End time.Time // first Sunday and last included day
	Weeks      [][7]Day
	Max        int // highest daily total
	Total      int // commits inside the calendar
}

// Calendar builds a grid of the given number of weeks ending on end's date.
// Only commits by author are counted unless author is AllAuthors.
func (d *Data) Calendar(end time.Time, weeks, author int) Calendar {
	if weeks < 1 {
		weeks = 1
	}
	end = midnight(end.Local())
	start := addDays(end, -int(end.Weekday())-7*(weeks-1))
	cal := Calendar{Start: start, End: end, Weeks: make([][7]Day, weeks)}
	for w := range cal.Weeks {
		for wd := 0; wd < 7; wd++ {
			date := addDays(start, w*7+wd)
			cal.Weeks[w][wd] = Day{Date: date, Top: -1, Future: date.After(end)}
		}
	}
	for _, c := range d.commits {
		if author != AllAuthors && c.author != author {
			continue
		}
		n := daysBetween(start, c.day)
		if n < 0 || c.day.After(end) {
			continue
		}
		day := &cal.Weeks[n/7][n%7]
		if day.ByAuthor == nil {
			day.ByAuthor = map[int]int{}
		}
		day.ByAuthor[c.author]++
		day.Total++
		cal.Total++
	}
	for w := range cal.Weeks {
		for wd := range cal.Weeks[w] {
			day := &cal.Weeks[w][wd]
			day.Top = topAuthor(day.ByAuthor)
			if day.Total > cal.Max {
				cal.Max = day.Total
			}
		}
	}
	return cal
}

// Level maps a daily total to an intensity from 0 (none) to 4 (busiest).
func (c Calendar) Level(total int) int { return level(total, c.Max) }

// Slot is one weekday-hour cell of a punch card.
type Slot struct {
	Total int
	Top   int // author with most commits in the slot, -1 when empty
}

// PunchCard counts commits by weekday (rows, Sunday first) and hour, read
// on each author's own clock: a commit made at 23:00 in Tokyo lands at 23
// wherever the history is viewed.
type PunchCard struct {
	Slots    [7][24]Slot
	ByAuthor map[int]*[7][24]int // each author's commits by weekday and hour
	Zones    map[int]int         // commits by UTC offset in seconds
	Max      int                 // highest slot total
	Total    int
}

// PunchCard builds the punch card of the commits between from and to
// (inclusive dates), counting only author's commits unless author is
// AllAuthors.
func (d *Data) PunchCard(from, to time.Time, author int) PunchCard {
	from, to = midnight(from.Local()), midnight(to.Local())
	card := PunchCard{ByAuthor: map[int]*[7][24]int{}, Zones: map[int]int{}}
	var byAuthor [7][24]map[int]int
	for _, c := range d.commits {
		if author != AllAuthors && c.author != author || c.day.Before(from) || c.day.After(to) {
			continue
		}
		wd, h := c.clock.Weekday(), c.clock.Hour()
		if byAuthor[wd][h] == nil {
			byAuthor[wd][h] = map[int]int{}
		}
		byAuthor[wd][h][c.author]++
		if card.ByAuthor[c.author] == nil {
			card.ByAuthor[c.author] = &[7][24]int{}
		}
		card.ByAuthor[c.author][wd][h]++
		_, offset := c.clock.Zone()
		card.Zones[offset]++
		card.Slots[wd][h].Total++
		card.Total++
	}
	for wd := range card.Slots {
		for h := range card.Slots[wd] {
			slot := &card.Slots[wd][h]
			slot.Top = topAuthor(byAuthor[wd][h])
			card.Max = max(card.Max, slot.Total)
		}
	}
	return card
}

// Level maps a slot total to an intensity from 0 (none) to 4 (busiest).
func (p PunchCard) Level(total int) int { return level(total, p.Max) }

func level(total, most int) int {
	if total <= 0 || most <= 0 {
		return 0
	}
	l := (4*total + most - 1) / most
	return min(max(l, 1), 4)
}

// topAuthor returns the author with the most commits, or -1 when there are
// none. Ties go to the higher-ranked (lower index) author.
func topAuthor(byAuthor map[int]int) int {
	top := -1
	for a, n := range byAuthor {
		if top < 0 || n > byAuthor[top] || (n == byAuthor[top] && a < top) {
			top = a
		}
	}
	return top
}

// AuthorCommits counts each author's commits between from and to (inclusive
// dates), indexed like Authors.
func (d *Data) AuthorCommits(from, to time.Time) []int {
	from, to = midnight(from.Local()), midnight(to.Local())
	counts := make([]int, len(d.Authors))
	for _, c := range d.commits {
		if !c.day.Before(from) && !c.day.After(to) {
			counts[c.author]++
		}
	}
	return counts
}

// FileStat is the change history of one path.
type FileStat struct {
	Path      string
	Commits   int
	Added     int
	Deleted   int
	Binary    bool
	Gone      bool // deleted: no longer in the tracked files
	TopAuthor int  // author with most commits touching the file
}

// FileRank returns the files changed in the most commits between from and to
// (inclusive dates), limited to limit entries when limit > 0.
func (d *Data) FileRank(from, to time.Time, author, limit int) []FileStat {
	from, to = midnight(from.Local()), midnight(to.Local())
	type acc struct {
		FileStat
		byAuthor map[int]int
	}
	files := map[string]*acc{}
	for _, c := range d.commits {
		if author != AllAuthors && c.author != author {
			continue
		}
		if c.day.Before(from) || c.day.After(to) {
			continue
		}
		for _, f := range c.files {
			a := files[f.Path]
			if a == nil {
				gone := d.tracked != nil && !d.tracked[f.Path]
				a = &acc{FileStat: FileStat{Path: f.Path, Gone: gone}, byAuthor: map[int]int{}}
				files[f.Path] = a
			}
			a.Commits++
			a.byAuthor[c.author]++
			if f.Added < 0 {
				a.Binary = true
			} else {
				a.Added += f.Added
				a.Deleted += f.Deleted
			}
		}
	}
	out := make([]FileStat, 0, len(files))
	for _, a := range files {
		a.TopAuthor = topAuthor(a.byAuthor)
		out = append(out, a.FileStat)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Commits != out[j].Commits {
			return out[i].Commits > out[j].Commits
		}
		ci, cj := out[i].Added+out[i].Deleted, out[j].Added+out[j].Deleted
		if ci != cj {
			return ci > cj
		}
		return out[i].Path < out[j].Path
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// FileCommit is one commit touching a file.
type FileCommit struct {
	Hash    string
	Subject string
	Author  int
	Time    time.Time
	Added   int // -1 for binary changes
	Deleted int
}

// FileCommits returns the commits touching the file at its latest path
// between from and to (inclusive dates), newest first, counted like
// FileRank.
func (d *Data) FileCommits(path string, from, to time.Time, author int) []FileCommit {
	from, to = midnight(from.Local()), midnight(to.Local())
	var out []FileCommit
	for _, c := range d.commits {
		if author != AllAuthors && c.author != author || c.day.Before(from) || c.day.After(to) {
			continue
		}
		for _, f := range c.files {
			if f.Path == path {
				out = append(out, FileCommit{
					Hash: c.hash, Subject: c.subject, Author: c.author, Time: c.time,
					Added: f.Added, Deleted: f.Deleted,
				})
			}
		}
	}
	return out
}

// resolveRenames returns each commit's file changes with paths rewritten to
// the file's latest name. Walking from newest to oldest, a rename maps the
// old path onto the new file, so earlier changes to the old path follow it,
// while later reuse of the old path stays separate.
func resolveRenames(commits []gitlog.Commit) [][]gitlog.FileChange {
	latest := map[string]string{} // old path -> latest path
	out := make([][]gitlog.FileChange, len(commits))
	for i, c := range commits {
		out[i] = make([]gitlog.FileChange, len(c.Files))
		for j, f := range c.Files {
			if p, ok := latest[f.Path]; ok {
				f.Path = p
			}
			if f.OldPath != "" {
				latest[f.OldPath] = f.Path
			}
			out[i][j] = f
		}
	}
	return out
}

// identityKeys returns the keys that tie commits to one author: the
// lower-cased email and the name.
func identityKeys(name, email string) []string {
	var ks []string
	if email != "" {
		ks = append(ks, "e:"+strings.ToLower(email))
	}
	if name != "" {
		ks = append(ks, "n:"+name)
	}
	if len(ks) == 0 {
		ks = append(ks, "unknown")
	}
	return ks
}

// groupIdentities assigns each commit a group id, joining commits that
// share a lower-cased email or a name.
func groupIdentities(commits []gitlog.Commit) []int {
	parent := map[string]string{}
	var find func(string) string
	find = func(k string) string {
		p, ok := parent[k]
		if !ok || p == k {
			parent[k] = k
			return k
		}
		root := find(p)
		parent[k] = root
		return root
	}
	keys := make([]string, len(commits))
	for i, c := range commits {
		ks := identityKeys(c.Author, c.Email)
		root := find(ks[0])
		for _, k := range ks[1:] {
			if r := find(k); r != root {
				parent[r] = root
			}
		}
		keys[i] = ks[0]
	}
	groups := map[string]int{}
	ids := make([]int, len(commits))
	for i, k := range keys {
		root := find(k)
		id, ok := groups[root]
		if !ok {
			id = len(groups)
			groups[root] = id
		}
		ids[i] = id
	}
	return ids
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func addDays(t time.Time, n int) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day()+n, 0, 0, 0, 0, t.Location())
}

// daysBetween counts calendar days from a to b, independent of DST shifts.
func daysBetween(a, b time.Time) int {
	ua := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	ub := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(ub.Sub(ua).Hours() / 24)
}
