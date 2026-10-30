// Command git-inktrace shows a contribution heatmap and file change ranking
// for a git repository in the terminal.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZhiWei-Ou/git-inktrace/internal/gitlog"
	"github.com/ZhiWei-Ou/git-inktrace/internal/stats"
	"github.com/ZhiWei-Ou/git-inktrace/internal/tui"
	"github.com/ZhiWei-Ou/git-inktrace/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "git-inktrace:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		dir         = flag.String("C", ".", "run as if started in `path`")
		all         = flag.Bool("all", false, "include every branch, remote branch and tag, not only HEAD (stashes excluded)")
		top         = flag.Int("top", 100, "number of files in the ranking (0 for all)")
		heatmap     = flag.Bool("heatmap", false, "open on the heatmap (default)")
		files       = flag.Bool("files", false, "open on the file ranking")
		graph       = flag.Bool("graph", false, "open on the branch graph")
		hours       = flag.Bool("hours", false, "open on the punch card of commit hours")
		owners      = flag.Bool("owners", false, "open on the code ownership treemap")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: git-inktrace [flags]\n\n"+
			"Show a contribution heatmap, the most frequently changed files, the\n"+
			"branch graph and a punch card of commit hours of the git repository\n"+
			"in the current directory.\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		return fmt.Errorf("unexpected argument %q", flag.Arg(0))
	}
	view, err := startView(*heatmap, *files, *graph, *hours, *owners)
	if err != nil {
		return err
	}
	if *showVersion {
		fmt.Println("git-inktrace", version.Get())
		return nil
	}

	root, err := gitlog.TopLevel(*dir)
	if err != nil {
		return err
	}
	// File changes take most of the reading time on a large history, so the
	// TUI opens on the headers and reads them in the background.
	commits, err := gitlog.ReadHeaders(root, gitlog.Options{All: *all})
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		return fmt.Errorf("no commits in %s", root)
	}
	tracked, err := gitlog.Tracked(root)
	if err != nil {
		return err
	}
	created, err := gitlog.FirstCommitTime(root)
	if err != nil {
		return err
	}
	branches, err := gitlog.BranchCount(root)
	if err != nil {
		return err
	}
	return tui.Run(stats.New(commits, tracked), tui.Options{
		Repo:     filepath.Base(root),
		Branch:   gitlog.Branch(root),
		Top:      *top,
		Dir:      root,
		All:      *all,
		Created:  created,
		Commits:  commits,
		Tracked:  tracked,
		Branches: branches,
		View:     view,
	})
}

// startView picks the first page from the -heatmap, -files, -graph, -hours
// and -owners flags.
func startView(heatmap, files, graph, hours, owners bool) (tui.View, error) {
	view, n := tui.HeatmapView, 0
	for _, f := range []struct {
		set  bool
		view tui.View
	}{{heatmap, tui.HeatmapView}, {files, tui.FilesView}, {graph, tui.GraphView}, {hours, tui.HoursView}, {owners, tui.OwnersView}} {
		if f.set {
			view, n = f.view, n+1
		}
	}
	if n > 1 {
		return 0, fmt.Errorf("-heatmap, -files, -graph, -hours and -owners are mutually exclusive")
	}
	return view, nil
}
