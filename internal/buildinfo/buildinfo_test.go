// SPDX-License-Identifier: MIT

package buildinfo

import (
	"runtime"
	"runtime/debug"
	"testing"
)

const sha = "1802d0f9afe8642d1e278120b76fcdfac967cb9e"

// with sets the injected values and the embedded build info for one test.
func with(t *testing.T, version, tag, commit, buildDate string, bi *debug.BuildInfo) {
	t.Helper()
	oldInjected, oldRead := injected, readBuildInfo
	t.Cleanup(func() { injected, readBuildInfo = oldInjected, oldRead })
	Set(version, tag, commit, buildDate)
	readBuildInfo = func() (*debug.BuildInfo, bool) { return bi, bi != nil }
}

func platform() string { return runtime.Version() + ", " + runtime.GOOS + "/" + runtime.GOARCH }

func TestStringAllFields(t *testing.T) {
	with(t, "3.5.1", "v3.5.1", sha, "2026-11-05T15:41:49Z", nil)
	want := "tool 3.5.1 (tag v3.5.1, commit " + sha + ", built 2026-11-05T15:41:49Z, " + platform() + ")"
	if got := Get("tool").String(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestStringWithoutTag(t *testing.T) {
	for _, tag := range []string{"", "none"} {
		with(t, "v3.5.1-2-gabcdef", tag, sha, "2026-11-05T15:41:49Z", nil)
		want := "tool 3.5.1-2-gabcdef (commit " + sha + ", built 2026-11-05T15:41:49Z, " + platform() + ")"
		if got := Get("tool").String(); got != want {
			t.Fatalf("tag %q: got  %q\nwant %q", tag, got, want)
		}
	}
}

func TestDevFallbackWithoutBuildInfo(t *testing.T) {
	with(t, "", "", "", "", nil)
	want := "tool dev (commit unknown, built unknown, " + platform() + ")"
	if got := Get("tool").String(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestDevFallbackPlaceholders(t *testing.T) {
	with(t, "dev", "none", "unknown", "unknown", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}})
	want := "tool dev (commit unknown, built unknown, " + platform() + ")"
	if got := Get("tool").String(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestDirtyCheckoutVersionIsDev(t *testing.T) {
	bi := &debug.BuildInfo{
		Main:     debug.Module{Version: "v0.4.1-0.20261107212139-415629327cbc+dirty"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: sha}, {Key: "vcs.modified", Value: "true"}},
	}
	with(t, "", "", "", "", bi)
	want := "tool dev (commit " + sha + "-dirty, built unknown, " + platform() + ")"
	if got := Get("tool").String(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestFallbackToEmbeddedBuildInfo(t *testing.T) {
	bi := &debug.BuildInfo{
		Main:     debug.Module{Version: "v3.5.1"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: sha}, {Key: "vcs.modified", Value: "true"}},
	}
	with(t, "", "", "", "", bi)
	want := "tool 3.5.1 (commit " + sha + "-dirty, built unknown, " + platform() + ")"
	if got := Get("tool").String(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	bi.Settings[1].Value = "false"
	if got := Get("tool").Commit; got != sha {
		t.Fatalf("clean tree: commit %q, want %q", got, sha)
	}
}

func TestInjectedWinsOverBuildInfo(t *testing.T) {
	bi := &debug.BuildInfo{
		Main:     debug.Module{Version: "v0.0.1"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "0000"}},
	}
	with(t, "v3.5.1", "v3.5.1", sha, "2026-11-05T15:41:49Z", bi)
	i := Get("tool")
	if i.Version != "3.5.1" || i.Commit != sha {
		t.Fatalf("got %+v", i)
	}
}

func TestJSONKeyOrderAndValues(t *testing.T) {
	with(t, "3.5.1", "v3.5.1", sha, "2026-11-05T15:41:49Z", nil)
	want := `{"name":"tool","version":"3.5.1","tag":"v3.5.1","commit":"` + sha +
		`","built":"2026-11-05T15:41:49Z","go":"` + runtime.Version() + `","platform":"` + runtime.GOOS + "/" + runtime.GOARCH + `"}`
	if got := Get("tool").JSON(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestJSONDevBuild(t *testing.T) {
	with(t, "", "", "", "", nil)
	want := `{"name":"tool","version":"dev","tag":"","commit":"unknown","built":"unknown","go":"` +
		runtime.Version() + `","platform":"` + runtime.GOOS + "/" + runtime.GOARCH + `"}`
	if got := Get("tool").JSON(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
