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
 git-inktrace  demo · main                                       Jan 4, 2026 – Oct 30, 2026
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

     Fri, Oct 16 2026 · 1 commit  ■ Ivan 1

     ■ Carol 22  ■ Dave 22  ■ Bob 24  ■ Grace 22  ■ Heidi 21  ■ Alice 22  ■ Erin 19
     ■ Ivan 20  ■ +1 other 17

 ←↓↑→ move · [ ] prev/next · t today · a/A author · tab files · q quit
```

## Features

- **Contribution heatmap** — a GitHub-style calendar of weeks by weekdays, sized to fit the terminal (up to 53 weeks).
- **A color per developer** — each day takes the color of the author with the most commits that day; shade shows how busy the day was. The top eight authors get their own hue, everyone else shares grey.
- **Hot file ranking** — files ordered by how many commits touched them, with lines added and deleted and the author who changed each file most.
- **Focus on one author** — filter both views to a single contributor; authors are merged by email, so name changes do not split them.
- **Browse history** — step the window back and forth through the years and inspect any day's commits by author.
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

The heatmap ends today. If the repository has had no commits for over a year, it ends on the latest commit instead. Merge commits are skipped, and renames count as a delete plus an add.

### Keys

| Key | Heatmap | Files |
| --- | --- | --- |
| `tab`, `1`, `2` | switch view | switch view |
| `←` `→` / `h` `l` | previous / next week | |
| `↑` `↓` / `k` `j` | previous / next day | scroll |
| `pgup` `pgdn` | | scroll a page |
| `g` `G` | first / last day | top / bottom |
| `[` `]` | previous / next time window | previous / next time window |
| `t` | back to the latest window | back to the latest window |
| `a` / `A` | next / previous author filter | next / previous author filter |
| `q`, `esc` | quit | quit |

The file ranking always covers the same dates and author filter as the heatmap.

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
