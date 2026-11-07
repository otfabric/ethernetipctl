// SPDX-License-Identifier: MIT

package main

import (
	"os"

	"github.com/otfabric/ethernetipctl/cmd"
)

// Injected at link time (see Makefile): -X main.version=... etc.
var (
	version   string
	tag       string
	commit    string
	buildDate string
)

// exit ends the process; a test replaces it.
var exit = os.Exit

func main() {
	cmd.SetBuildMeta(version, tag, commit, buildDate)
	exit(cmd.Execute())
}
