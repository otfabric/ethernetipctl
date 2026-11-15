// SPDX-License-Identifier: MIT

package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// routeEnv is the environment variable of --route (config.RouteConfig).
const routeEnv = "ETHERNETIPCTL_ROUTE"

// RouteFromFlagOnly makes --route of cmd, a command that changes a device, a flag of the
// command line only: the default ETHERNETIPCTL_ROUTE gave it when the flags were registered
// is cleared, and its help no longer names the variable, which the reading commands go on
// using. Call it after the flags are registered.
func RouteFromFlagOnly(cmd *cobra.Command) {
	f := cmd.Flags().Lookup("route")
	if f == nil {
		return
	}
	_ = f.Value.Set("")
	f.DefValue = ""
	f.Usage = strings.TrimSuffix(f.Usage, " [env: "+routeEnv+"]") + " (only from the command line: a change does not take " + routeEnv + ")"
}

// NoteEnvRouteNotUsed writes one line to the stderr of cmd, a command that changes a
// device, when ETHERNETIPCTL_ROUTE is set and --route is not given: the command does not
// use the variable, and goes to the host on the command line.
func NoteEnvRouteNotUsed(cmd *cobra.Command) {
	r := strings.TrimSpace(os.Getenv(routeEnv))
	if r == "" || cmd.Flags().Changed("route") {
		return
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s=%s is not used: a change takes its route only from --route\n", routeEnv, r)
}
