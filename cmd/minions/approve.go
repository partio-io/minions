package main

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"github.com/partio-io/minions/internal/approve"
)

func newApproveCmd() *cobra.Command {
	var (
		dryRun bool
		delay  string
		repo   string
	)

	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Auto-approve eligible proposal issues",
		Long:  `Scans open minion-proposal issues and adds the minion-approved label to those that have passed the review window without objection.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			delayDur, err := time.ParseDuration(delay)
			if err != nil {
				return fmt.Errorf("parsing delay %q: %w", delay, err)
			}

			slog.Info("listing proposals", "repo", repo, "delay", delayDur)
			issues, err := approve.ListProposals(repo)
			if err != nil {
				return err
			}

			if len(issues) == 0 {
				fmt.Println("No open proposals found.")
				return nil
			}

			fmt.Printf("Found %d open proposal(s)\n\n", len(issues))

			approved := 0
			for _, issue := range issues {
				ok, reason := approve.ShouldApprove(issue, delayDur)
				if !ok {
					fmt.Printf("  #%-5d SKIP  %s — %s\n", issue.Number, issue.Title, reason)
					continue
				}

				if dryRun {
					fmt.Printf("  #%-5d WOULD APPROVE  %s\n", issue.Number, issue.Title)
					approved++
					continue
				}

				if err := approve.Approve(repo, issue.Number); err != nil {
					slog.Error("approving issue", "issue", issue.Number, "error", err)
					continue
				}

				fmt.Printf("  #%-5d APPROVED  %s\n", issue.Number, issue.Title)
				approved++
			}

			fmt.Printf("\n%d issue(s) approved", approved)
			if dryRun {
				fmt.Print(" (dry-run)")
			}
			fmt.Println()

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be approved without making changes")
	cmd.Flags().StringVar(&delay, "delay", "24h", "minimum age before auto-approval (e.g., 24h, 0h)")
	cmd.Flags().StringVar(&repo, "repo", "partio-io/minions", "GitHub repository for proposal issues")

	return cmd
}
