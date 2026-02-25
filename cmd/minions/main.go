package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/partio-io/minions/internal/config"
	plog "github.com/partio-io/minions/internal/log"
)

var (
	version     = "dev"
	cfgLogLevel string
	cfg         config.Config
)

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "minions",
		Short: "One-shot coding agents for Partio repos",
		Long:  `minions orchestrates unattended Claude Code agents that generate PRs across all Partio repos.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg = config.Load()

			if cfgLogLevel != "" {
				cfg.LogLevel = cfgLogLevel
			}

			plog.Setup(cfg.LogLevel)
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&cfgLogLevel, "log-level", "", "log level (debug, info, warn, error)")

	root.AddCommand(
		newVersionCmd(),
		newRunCmd(),
		newIngestCmd(),
		newDocCmd(),
		newReadmeCmd(),
		newProposeCmd(),
		newApproveCmd(),
	)

	return root
}
