<h1 align="center">git-inktrace</h1>

<p align="center">A terminal UI that shows who worked on a git repository, when, and which files change the most.</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
</p>

<p align="center">
  <a href="#usage">Usage</a> ·
  <a href="https://github.com/ZhiWei-Ou/git-inktrace/issues">Issues</a>
</p>

```text
 git-inktrace  demo · main                                     ← Jan 4, 2026 – Oct 30, 2026
  Heatmap   Files                                                 189 commits · all authors

     Jan     Feb     Mar       Apr     May       Jun     Jul     Aug       Sep     Oct
     ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
 Mon ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
     ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
 Wed ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
     ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
 Fri ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
     ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■

     Less ■ ■ ■ ■ ■ More                                                    144 active days

       author                  this window      all history
     ■ Bob <bob@x.io>            24  12.7%        27  11.9%
     ■ Carol <carol@x.io>        22  11.6%        29  12.8%
     ■ Dave <dave@x.io>          22  11.6%        28  12.3%
     ■ Grace <grace@x.io>        22  11.6%        26  11.5%
     ■ Alice <alice@x.io>        22  11.6%        24  10.6%
     ■ Heidi <heidi@x.io>        21  11.1%        26  11.5%
     ■ Ivan <ivan@x.io>          20  10.6%        23  10.1%
     ■ Erin <erin@x.io>          19  10.1%        23  10.1%
     ■ +1 other                  17   9.0%        21   9.3%

 ←→ year · t latest · a/A author · tab files · q quit
```

## Features

- **Contribution heatmap** — a GitHub-style calendar of weeks by weekdays, sized to fit the terminal (up to 53 weeks).
- **Contribution share** — the legend lists each author with their email, plus commit count and share both in the visible time window and across the whole repository history, busiest first. When the window already holds every commit, only the history column is shown.
- **A color per developer** — each day takes the color of the author with the most commits that day; shade shows how busy the day was. The top eight authors get their own hue, everyone else shares grey.
- **Hot file ranking** — files ordered by how many commits touched them, with lines added and deleted and the author who changed each file most.
- **One person, one author** — commits are merged by email and by name, after applying `.mailmap`, so renames and multiple addresses do not split a contributor. Each author is shown with their newest name.
- **Focus on one author** — filter both views to a single contributor.
- **Browse history by year** — step the window back and forth one year at a time with the arrow keys; arrows in the title bar show which way more history lies.
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
  -all        include commits from all refs, not only HEAD
  -top int    number of files in the ranking, 0 for all (default 100)
  -version    print version and exit
```

Examples:

```sh
git inktrace -C ~/src/project     # inspect another repository
git inktrace -all                 # count commits on every branch and tag
git inktrace -top 0               # rank every changed file
```

Authors are merged the way `git shortlog` groups them, plus by email: commits that share an email (case-insensitive) or a name belong to one author. To join identities that share neither, add a [`.mailmap`](https://git-scm.com/docs/gitmailmap) file to the repository, for example:

```text
Alice Liddell <alice@wonderland.org> <alice@old-laptop.local>
Alice Liddell <alice@wonderland.org> Ally <ally@example.com>
```

The heatmap ends today. If the repository has had no commits for over a year, it ends on the latest commit instead. Merge commits are skipped, and renames count as a delete plus an add.

### Keys

| Key | Action |
| --- | --- |
| `tab`, `1`, `2` | Switch between the heatmap and the file ranking |
| `←` `→` / `h` `l` | Move the window one year back / forward |
| `t` | Return to the latest window |
| `a` / `A` | Next / previous author filter |
| `↑` `↓` / `k` `j` | Scroll the file ranking |
| `pgup` `pgdn`, `g` `G` | Page, or jump to the top / bottom of the file ranking |
| `q`, `esc` | Quit |

The heatmap is read-only. The file ranking always covers the same dates and author filter as the heatmap. You can move back until the window reaches the first commit, and forward until the latest window.

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
