<h1 align="center">git-inktrace</h1>

<p align="center">A terminal UI that shows who worked on a git repository, when — down to the hour — which files change the most, and how branches evolved.</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
</p>

<p align="center">
  <a href="#usage">Usage</a> ·
  <a href="https://github.com/ZhiWei-Ou/git-inktrace/issues">Issues</a>
</p>

```text
 git-inktrace  demo · main                                      ← Dec 21, 2025 – Oct 30, 2026
  Heatmap   Files                                                     194 commits · all authors

     Dec Jan     Feb     Mar       Apr     May       Jun     Jul     Aug       Sep     Oct
     ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
 Mon ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
     ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
 Wed ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
     ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
 Fri ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
     ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■

     Less ■ ■ ■ ■ ■ More                                                        148 active days

       author                     Oct 2026      this window      all history
     ■ Grace <Grace@x.io>         7  26.9%        23  11.9%        26  11.5%
     ■ Carol <Carol@x.io>         4  15.4%        23  11.9%        29  12.8%
     ■ Heidi <Heidi@x.io>         3  11.5%        22  11.3%        26  11.5%
     ■ Bob <Bob@x.io>             2   7.7%        24  12.4%        27  11.9%
     ■ Alice <Alice@x.io>         2   7.7%        22  11.3%        24  10.6%
     ■ Ivan <Ivan@x.io>           2   7.7%        21  10.8%        23  10.1%
     ■ Erin <Erin@x.io>           2   7.7%        19   9.8%        23  10.1%
     ■ Dave <Dave@x.io>           1   3.8%        22  11.3%        28  12.3%
     ■ +1 other                   3  11.5%        18   9.3%        21   9.3%

 ←→ year   a author   tab next page   ? help   q quit
```

## Features

- **Contribution heatmap** — a GitHub-style calendar of weeks by weekdays, sized to fit the terminal (up to 53 weeks).
- **Repository overview** — above the heatmap, aligned columns show when the repository was created (its first commit on any branch, remote branch or tag, not only the checked-out one), how old it is, when the last commit landed, and the numbers of commits and authors. Columns that do not fit the terminal are left out.
- **Contribution share** — the legend lists each author with their email, with commit count and share in the current calendar month, the visible time window and the whole repository history, busiest first. A column that would repeat its neighbor is hidden — for example, only the history column is shown when the window already holds every commit.
- **A color per developer** — each day takes the color of the author with the most commits that day; shade shows how busy the day was. The top eight authors get their own hue, everyone else shares grey.
- **Hot file ranking** — files ordered by how many commits touched them, with lines added and deleted and the author who changed each file most. History follows renames to each file's current path, and files no longer in `HEAD` are tagged `(deleted)`. Select a file and press `enter` to list the commits that touched it.
- **Branch graph** — the commit graph drawn by git (`git log --graph`) with branches, tags and merges: lanes keep git's branch colors (configurable with `log.graphColors`) and each commit's `*` takes its author's color. It shows every branch, remote branch and tag by default (stashes excluded); press `a` to switch to the current branch and back. Select a commit and press `enter` to read its full message and the files it changed, like `git diff --stat`, `o` to copy its full hash to the system clipboard, or `e` to reword its message; press `/` to search commits by message, hash, author or email; history loads page by page as you scroll.
- **Reword a commit message** — press `e` on a commit of the current branch to edit its message in the editor git would use (`GIT_EDITOR`, `core.editor`, `VISUAL`, `EDITOR`, then `vi`). When a remote-tracking branch already holds the commit, git-inktrace warns that rewording it needs a force push and continues only on `y`. The commits after it are recreated with the same trees, authors and dates and merges keep their shape, so the work tree, the index and uncommitted changes stay untouched and no conflict can arise; the current branch moves to the rewritten history, while other branches keep the original commits. This is the only action that writes to the repository.
- **Punch card** — commits by weekday and hour, read on each author's own clock, so a commit made at 23:00 in Tokyo counts at 23:00 wherever you look at it. Each slot takes the color of its busiest author and grows with how busy it was; weekday totals sit on the right and an hourly profile below, all centered. Above it, aligned columns show the peak slot, the busiest hour, the shares of commits made after hours (weekdays before 9:00 or from 18:00) and on weekends, and the main time zone with how many there are. Below it, each author gets a row with their own hourly profile, busiest hour and after-hours and weekend shares.
- **One person, one author** — commits are merged by email and by name, after applying `.mailmap`, so renames and multiple addresses do not split a contributor. Each author is shown with their newest name.
- **Focus on one author** — filter the heatmap, the file ranking and the punch card to a single contributor.
- **Browse history by year** — step the window back and forth with the arrow keys; arrows in the title bar show which way more history lies.
- **Works as a git subcommand** — put `git-inktrace` on your `PATH` and run `git inktrace` in any repository.

## Quick Start

Requires Go 1.26 or later, `git` and `make`.

```sh
git clone https://github.com/ZhiWei-Ou/git-inktrace.git
cd git-inktrace
make build
./bin/git-inktrace
```

The TUI opens on the git-inktrace repository itself. Press `tab` to switch to the file ranking and `q` to quit.

To use it anywhere, copy `bin/git-inktrace` to a directory on your `PATH`, then run it from inside any git repository:

```sh
cd path/to/your/repo
git inktrace
```

## Usage

```text
git-inktrace [flags]

  -C path     run as if started in path (default ".")
  -all        include every branch, remote branch and tag, not only HEAD (stashes excluded)
  -top int    number of files in the ranking, 0 for all (default 100)
  -heatmap    open on the heatmap (default)
  -files      open on the file ranking
  -graph      open on the branch graph
  -hours      open on the punch card of commit hours
  -version    print version and exit
```

Examples:

```sh
git inktrace -C ~/src/project     # inspect another repository
git inktrace -all                 # count commits on every branch and tag, skipping stashes
git inktrace -top 0               # rank every changed file
git inktrace -graph               # open straight on the branch graph
git inktrace -hours               # see when the team commits
```

Authors are merged the way `git shortlog` groups them, plus by email: commits that share an email (case-insensitive) or a name belong to one author. To join identities that share neither, add a [`.mailmap`](https://git-scm.com/docs/gitmailmap) file to the repository, for example:

```text
Alice Liddell <alice@wonderland.org> <alice@old-laptop.local>
Alice Liddell <alice@wonderland.org> Ally <ally@example.com>
```

The heatmap ends today. If the repository has had no commits for over a year, it ends on the latest commit instead. Merge commits are skipped. Renames detected by git (`git log -M`) carry a file's history to its latest path; a file deleted in a later commit shows equal added and deleted totals, because every line it ever had was eventually removed.

### Keys

| Key | Action |
| --- | --- |
| `?` | Show or hide every key, grouped by page; the bottom line keeps only the main ones |
| `tab`, `1`, `2`, `3`, `4` | Switch between the heatmap, the file ranking, the graph and the punch card |
| `←` `→` / `h` `l` | Move the window one year back / forward (one window at a time when the terminal shows less than a year) |
| `t` | Return to the latest window |
| `a` / `A` | Next / previous author filter (heatmap, file ranking and punch card) |
| `↑` `↓` / `k` `j` | Select the previous / next file in the ranking, commit in the graph, or scroll an inspect page |
| `enter` | Inspect the selected file (the commits that touched it) or commit (its full message and the files it changed); `q` or `esc` goes back |
| `o` | Copy the selected commit's full hash to the clipboard (graph) |
| `e` | Reword the selected commit's message in your editor (graph, current branch only); `y` confirms when the commit is already pushed |
| `/` | Search the graph, highlighting matches as you type; `tab` completes `@commit`, `@author` or `@email`, `enter` jumps to the first match from the selection, `esc` closes the prompt and clears the highlight, as does an empty search; outside the prompt `esc` clears the last search |
| `n` / `N` | Jump to the next / previous matching commit (graph) |
| `pgup` `pgdn`, `g` `G` | Page, or jump to the top / bottom of the file ranking or the graph |
| `a` (graph) | Switch the graph between all branches (default) and the current branch |
| `q`, `ctrl+c` | Quit (`q` leaves an inspect page first) |

The heatmap is read-only. The file ranking, and the commits listed when inspecting a file, always cover the same dates and author filter as the heatmap. Inspecting a merge commit compares it with its first parent. You can move back while older commits lie before the window, and forward until the latest window. The month column is the calendar month of the window's last day.

The year keys apply to the heatmap and file ranking; the graph shows the whole history. In the graph, an author filter chosen in the other views fades other authors' commits, and authors not counted in the heatmap (for example, on unmerged branches without `-all`) are shown in grey. A search matches the commit message by default, case-insensitively. Start a part with `@commit` to match a hash prefix, `@author` to match the author's name or `@email` to match the email; every part must match, for example `fix parser @author bob @email example.com`. Matching text is highlighted in loaded commits; an `@email` match highlights the author. `n` and `N` stop at the last and first match; past the last loaded match, `n` loads older history so you can press it again. Copying uses the system clipboard: `pbcopy` on macOS, the Windows API on Windows, and `xclip`, `xsel` or `wl-copy` on Linux, one of which must be installed.

```text
 * 0423a4e (HEAD -> main) release prep                             ■ Alice           2026-10-30
 | * 320eea5 (wip) experiment                                      ■ Dave            2026-10-30
 |/
 * 2941c72 (tag: v0.1.0) Merge branch 'feat'                       ■ Alice           2026-10-30
 |\
 | * 88e70f0 (feat) parser tests                                   ■ Bob             2026-10-30
 | * b4893bc add parser                                            ■ Bob             2026-10-30
 * | 2bb4a49 fix docs                                              ■ Carol           2026-10-30
 |/
 * 1db4d7c core setup                                              ■ Alice           2026-10-30
```

### Building

All build rules live in the `Makefile`. Go workspaces are turned off (`GOWORK=off`), so a parent `go.work` never affects the build.

| Target | Description |
| --- | --- |
| `make build` | Build for the host platform into `bin/git-inktrace` |
| `make install` | Install `git-inktrace` into `$(go env GOBIN)`, or `$(go env GOPATH)/bin` if unset |
| `make test` | Run unit tests |
| `make check` | Run `go vet` and the tests |
| `make release` | Build static release binaries for every platform into `bin/`, plus a SHA-256 checksum file |
| `make release-<os>-<arch>` | Build one release binary, e.g. `make release-linux-arm64` |
| `make clean` | Remove `bin/` |

Release platforms: `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `linux/arm` (ARMv7), `windows/amd64`, `windows/arm64`. Release binaries are named `git-inktrace-<version>-<os>-<arch>`, with `.exe` on Windows.

### Versioning

The version comes from git: the Makefile injects `git describe --tags --always --dirty`, and the Go toolchain embeds the commit revision, commit time and dirty flag (`-buildvcs`). `git-inktrace -version` prints all of them:

```text
git-inktrace v0.1.0 (rev 68b0c4dadda0, 2026-10-30T01:43:57Z, darwin/arm64)
```

Tag a commit (`git tag v0.1.0`) before `make release` to get a release version instead of a commit hash.

## Contributing

Bug reports and ideas are welcome in [GitHub Issues](https://github.com/ZhiWei-Ou/git-inktrace/issues). For code changes, run `make check` before opening a pull request.

## License

[MIT](LICENSE)
