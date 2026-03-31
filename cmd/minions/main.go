package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/partio-io/minions/internal/config"
	plog "github.com/partio-io/minions/internal/log"
	"github.com/partio-io/minions/internal/project"
)

var (
	version        = "dev"
	cfgLogLevel    string
	cfgProjectFile string
	cfg            config.Config
	proj           *project.Project // nil if no project.yaml found
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
		Short: "One-shot coding agents across repositories",
		Long:  `minions orchestrates unattended Claude Code agents that generate PRs across repositories.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg = config.Load()

			if cfgLogLevel != "" {
				cfg.LogLevel = cfgLogLevel
			}
			if cfgProjectFile != "" {
				cfg.ProjectFile = cfgProjectFile
			}

			plog.Setup(cfg.LogLevel)

			// Load project config
			if cfg.ProjectFile != "" {
				p, err := project.Load(cfg.ProjectFile)
				if err != nil {
					return fmt.Errorf("loading project config: %w", err)
				}
				proj = p
			} else {
				// Auto-discover from workspace root
				wsRoot := cfg.WorkspaceRoot
				if wsRoot == "" {
					if wd, err := os.Getwd(); err == nil {
						wsRoot = filepath.Dir(wd)
					}
				}
				if wsRoot != "" {
					proj = project.Discover(wsRoot)
				}
			}

			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&cfgLogLevel, "log-level", "", "log level (debug, info, warn, error)")
	root.PersistentFlags().StringVar(&cfgProjectFile, "project", "", "path to .minions/project.yaml")

	root.AddCommand(
		newVersionCmd(),
		newRunCmd(),
		newInitCmd(),
	)

	return root
}
