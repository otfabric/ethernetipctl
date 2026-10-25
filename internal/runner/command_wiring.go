package runner

import (
	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/spf13/cobra"
)

// RegisterCommandCfg runs [config.MustLoadFromEnv] and [config.RegisterFlags] for the
// flags of one command, then the optional hooks (extra completions, required flags, etc.).
func RegisterCommandCfg(cmd *cobra.Command, cfg any, hooks ...func(*cobra.Command)) {
	config.MustLoadFromEnv(cfg)
	config.RegisterFlags(cmd, cfg)
	for _, h := range hooks {
		if h != nil {
			h(cmd)
		}
	}
}
