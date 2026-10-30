<h1 align="center">git-inktrace</h1>

<p align="center">A terminal UI that shows who worked on a git repository, when, which files change the most, and how branches evolved.</p>

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

 ←→ year · t latest · a/A author · tab files · q quit
```

## Features

- **Contribution heatmap** — a GitHub-style calendar of weeks by weekdays, sized to fit the terminal (up to 53 weeks).
- **Contribution share** — the legend lists each author with their email, with commit count and share in the current calendar month, the visible time window and the whole repository history, busiest first. A column that would repeat its neighbor is hidden — for example, only the history column is shown when the window already holds every commit.
- **A color per developer** — each day takes the color of the author with the most commits that day; shade shows how busy the day was. The top eight authors get their own hue, everyone else shares grey.
- **Hot file ranking** — files ordered by how many commits touched them, with lines added and deleted and the author who changed each file most. History follows renames to each file's current path, and files no longer in `HEAD` are tagged `(deleted)`.
- **Branch graph** — the commit graph drawn by git (`git log --graph`) with branches, tags and merges: lanes keep git's branch colors (configurable with `log.graphColors`) and each commit's `*` takes its author's color. Toggle between `HEAD` and all branches; history loads page by page as you scroll.
- **One person, one author** — commits are merged by email and by name, after applying `.mailmap`, so renames and multiple addresses do not split a contributor. Each author is shown with their newest name.
- **Focus on one author** — filter both views to a single contributor.
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
  -version    print version and exit
```

Examples:

```sh
git inktrace -C ~/src/project     # inspect another repository
git inktrace -all                 # count commits on every branch and tag, skipping stashes
git inktrace -top 0               # rank every changed file
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
| `tab`, `1`, `2`, `3` | Switch between the heatmap, the file ranking and the graph |
| `←` `→` / `h` `l` | Move the window one year back / forward (one window at a time when the terminal shows less than a year) |
| `t` | Return to the latest window |
| `a` / `A` | Next / previous author filter |
| `↑` `↓` / `k` `j` | Scroll the file ranking or the graph |
| `pgup` `pgdn`, `g` `G` | Page, or jump to the top / bottom of the file ranking or the graph |
| `b` | Graph only `HEAD`, or every branch, remote branch and tag |
| `q`, `esc` | Quit |

The heatmap is read-only. The file ranking always covers the same dates and author filter as the heatmap. You can move back while older commits lie before the window, and forward until the latest window. The month column is the calendar month of the window's last day.

The year keys apply to the heatmap and file ranking; the graph shows the whole history. In the graph, the author filter fades other authors' commits, and authors not counted in the heatmap (for example, on unmerged branches without `-all`) are shown in grey. The graph starts with all branches when run with `-all`.

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
