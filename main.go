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
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: git-inktrace [flags]\n\n"+
			"Show a contribution heatmap and the most frequently changed files\n"+
			"of the git repository in the current directory.\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		return fmt.Errorf("unexpected argument %q", flag.Arg(0))
	}
	view, err := startView(*heatmap, *files, *graph)
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
	commits, err := gitlog.Read(root, gitlog.Options{All: *all})
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
	return tui.Run(stats.New(commits, tracked), tui.Options{
		Repo:   filepath.Base(root),
		Branch: gitlog.Branch(root),
		Top:    *top,
		Dir:    root,
		View:   view,
	})
}

// startView picks the first page from the -heatmap, -files and -graph flags.
func startView(heatmap, files, graph bool) (tui.View, error) {
	view, n := tui.HeatmapView, 0
	for _, f := range []struct {
		set  bool
		view tui.View
	}{{heatmap, tui.HeatmapView}, {files, tui.FilesView}, {graph, tui.GraphView}} {
		if f.set {
			view, n = f.view, n+1
		}
	}
	if n > 1 {
		return 0, fmt.Errorf("-heatmap, -files and -graph are mutually exclusive")
	}
	return view, nil
}
