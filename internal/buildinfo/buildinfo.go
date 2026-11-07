// SPDX-License-Identifier: MIT

// Package buildinfo is the build metadata of the binary, as the version command prints
// it: the values the release build injects with -ldflags -X main.*, completed from the
// module and VCS information Go embeds (runtime/debug.ReadBuildInfo).
package buildinfo

import (
	"encoding/json"
	"runtime"
	"runtime/debug"
	"strings"
)

// Info is the build metadata. Every field is set: an unknown value is "dev" (Version),
// "unknown" (Commit, Built) or "" (Tag only).
type Info struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Tag      string `json:"tag"`
	Commit   string `json:"commit"`
	Built    string `json:"built"`
	Go       string `json:"go"`
	Platform string `json:"platform"`
}

// injected holds the values main passes to Set (from -ldflags -X main.*).
var injected struct {
	version, tag, commit, buildDate string
}

// readBuildInfo is debug.ReadBuildInfo; a test replaces it.
var readBuildInfo = debug.ReadBuildInfo

// Set records the values injected at build time. Empty values and the placeholders
// "dev", "none" and "unknown" count as not injected.
func Set(version, tag, commit, buildDate string) {
	injected.version, injected.tag, injected.commit, injected.buildDate = version, tag, commit, buildDate
}

// Get returns the build metadata of the binary called name.
func Get(name string) Info {
	info := Info{
		Name:     name,
		Version:  strings.TrimPrefix(clean(injected.version), "v"),
		Tag:      clean(injected.tag),
		Commit:   clean(injected.commit),
		Built:    clean(injected.buildDate),
		Go:       runtime.Version(),
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}
	bi, ok := readBuildInfo()
	if ok && bi != nil {
		if info.Version == "" {
			if v := clean(bi.Main.Version); realVersion(v) {
				info.Version = strings.TrimPrefix(v, "v")
			}
		}
		if info.Commit == "" {
			info.Commit = vcsCommit(bi.Settings)
		}
	}
	if info.Version == "" {
		info.Version = "dev"
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	if info.Built == "" {
		info.Built = "unknown"
	}
	return info
}

// String is the one-line text form:
//
//	<name> <version> (tag <tag>, commit <commit>, built <built>, <go>, <goos>/<goarch>)
//
// The "tag <tag>, " part is left out when there is no tag.
func (i Info) String() string {
	var b strings.Builder
	b.WriteString(i.Name + " " + i.Version + " (")
	if i.Tag != "" {
		b.WriteString("tag " + i.Tag + ", ")
	}
	b.WriteString("commit " + i.Commit + ", built " + i.Built + ", " + i.Go + ", " + i.Platform + ")")
	return b.String()
}

// JSON is the one-line JSON form, keys in the order name, version, tag, commit, built,
// go, platform.
func (i Info) JSON() string {
	data, err := json.Marshal(i)
	if err != nil { // a struct of strings always encodes
		panic(err)
	}
	return string(data)
}

// vcsCommit is vcs.revision, with "-dirty" when vcs.modified is true, or "".
func vcsCommit(settings []debug.BuildSetting) string {
	var rev string
	var dirty bool
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return ""
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}

// realVersion reports whether v, the module version Go embeds, names a release: not
// "(devel)", and not the "+dirty" version Go stamps on a build of a modified checkout.
// go install …@vX.Y.Z embeds vX.Y.Z.
func realVersion(v string) bool {
	return v != "" && v != "(devel)" && !strings.HasSuffix(v, "+dirty")
}

// clean trims s and maps the placeholders of an uninjected value to "".
func clean(s string) string {
	s = strings.TrimSpace(s)
	switch s {
	case "dev", "none", "unknown":
		return ""
	}
	return s
}
