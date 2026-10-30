// Package version reports build version information.
//
// The revision, commit time and dirty flag come from the VCS metadata the Go
// toolchain embeds at build time (-buildvcs). Version may be overridden with
// -ldflags "-X github.com/ZhiWei-Ou/git-inktrace/internal/version.Version=...".
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Version is the release version, injected by the Makefile from `git describe`.
var Version = ""

// Info is the resolved build information.
type Info struct {
	Version  string
	Revision string
	Time     string
	Modified bool
}

// Get resolves the build information from ldflags and embedded VCS settings.
func Get() Info {
	info := Info{Version: Version}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		if info.Version == "" {
			info.Version = "dev"
		}
		return info
	}
	if info.Version == "" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		info.Version = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Revision = s.Value
		case "vcs.time":
			info.Time = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
	if info.Version == "" {
		info.Version = "dev"
	}
	return info
}

// String formats the build information as a single line.
func (i Info) String() string {
	var extra []string
	if i.Revision != "" {
		rev := i.Revision
		if len(rev) > 12 {
			rev = rev[:12]
		}
		if i.Modified {
			rev += "-dirty"
		}
		extra = append(extra, "rev "+rev)
	}
	if i.Time != "" {
		extra = append(extra, i.Time)
	}
	extra = append(extra, runtime.GOOS+"/"+runtime.GOARCH)
	return fmt.Sprintf("%s (%s)", i.Version, strings.Join(extra, ", "))
}
