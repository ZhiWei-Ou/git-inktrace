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
		all         = flag.Bool("all", false, "include commits from all refs, not only HEAD")
		top         = flag.Int("top", 100, "number of files in the ranking (0 for all)")
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
	return tui.Run(stats.New(commits), tui.Options{
		Repo:   filepath.Base(root),
		Branch: gitlog.Branch(root),
		Top:    *top,
	})
}
