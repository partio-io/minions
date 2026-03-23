package main

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/partio-io/minions/internal/propose"
)

func newProposeCmd() *cobra.Command {
	var (
		dryRun     bool
		sourceName string
		sourcesFile string
	)

	cmd := &cobra.Command{
		Use:   "propose",
		Short: "Check changelogs and create proposal issues",
		Long:  `Scans monitored changelog sources for new versions, extracts features via Claude, and creates proposal issues on GitHub.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			if sourcesFile == "" {
				// Default: sources.yaml in the minions repo root
				sourcesFile = filepath.Join(cfg.WorkspaceRoot, "minions", "sources.yaml")
			}

			slog.Info("loading sources", "file", sourcesFile)
			sources, err := propose.LoadSources(sourcesFile)
			if err != nil {
				return err
			}

			// The repo where proposal issues are created
			const issueRepo = "partio-io/minions"

			for i, src := range sources.Sources {
				if sourceName != "" && src.Name != sourceName {
					continue
				}

				latestVersion, err := propose.ProcessSource(ctx, src, issueRepo, dryRun)
				if err != nil {
					slog.Error("processing source failed", "source", src.Name, "error", err)
					continue
				}

				// Update last_version for next run
				sources.Sources[i].LastVersion = latestVersion
			}

			if !dryRun {
				if err := propose.SaveSources(sourcesFile, sources); err != nil {
					return fmt.Errorf("saving sources: %w", err)
				}
				slog.Info("updated sources.yaml")
			}

			fmt.Println("Done.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be created without making API calls")
	cmd.Flags().StringVar(&sourceName, "source", "", "process only this source (by name)")
	cmd.Flags().StringVar(&sourcesFile, "sources-file", "", "path to sources.yaml (default: <workspace>/minions/sources.yaml)")

	return cmd
}
