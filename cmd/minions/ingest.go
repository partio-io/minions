package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/partio-io/minions/internal/ingest"
)

func newIngestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Generate task specs from external sources",
		Long:  `Ingest features from changelogs, blog posts, or GitHub issues and generate task YAML files.`,
	}

	cmd.AddCommand(
		newIngestChangelogCmd(),
		newIngestBlogCmd(),
		newIngestIssuesCmd(),
	)

	return cmd
}

func newIngestChangelogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "changelog <url> [version]",
		Short: "Parse a changelog for feature ideas",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			url := args[0]
			version := ""
			if len(args) > 1 {
				version = args[1]
			}

			fmt.Printf("Ingesting changelog: %s\n", url)
			if version != "" {
				fmt.Printf("Version filter: %s\n", version)
			}

			content, err := ingest.FetchChangelog(url)
			if err != nil {
				return err
			}
			if content == "" {
				return fmt.Errorf("empty changelog from %s", url)
			}

			if version != "" {
				content = ingest.ExtractVersion(content, version)
				if content == "" {
					return fmt.Errorf("version %s not found in changelog", version)
				}
			}

			fmt.Printf("Changelog content (%d chars). Sending to Claude for analysis...\n", len(content))

			tasksDir := resolveTasksDir()
			sourceRef := fmt.Sprintf("%s (%s)", url, firstNonEmpty(version, "all"))
			count, err := ingest.GenerateTasks(ctx, "changelog", sourceRef, content, tasksDir)
			if err != nil {
				return err
			}

			fmt.Printf("\nDone. %d task spec(s) written to %s/\n", count, tasksDir)
			return nil
		},
	}
}

func newIngestBlogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "blog <url>",
		Short: "Extract ideas from a blog post",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			url := args[0]

			fmt.Printf("Ingesting blog post: %s\n", url)

			content, err := ingest.FetchBlog(url)
			if err != nil {
				return err
			}
			if content == "" {
				return fmt.Errorf("empty content from %s", url)
			}

			fmt.Printf("Blog content (%d chars). Sending to Claude for analysis...\n", len(content))

			tasksDir := resolveTasksDir()
			count, err := ingest.GenerateTasks(ctx, "blog", url, content, tasksDir)
			if err != nil {
				return err
			}

			fmt.Printf("\nDone. %d task spec(s) written to %s/\n", count, tasksDir)
			return nil
		},
	}
}

func newIngestIssuesCmd() *cobra.Command {
	var label string

	cmd := &cobra.Command{
		Use:   "issues <repo>",
		Short: "Convert labeled GitHub issues to task specs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := args[0]

			fmt.Printf("Ingesting issues from %s with label '%s'\n", repo, label)

			issues, err := ingest.FetchIssues(repo, label)
			if err != nil {
				return err
			}

			if len(issues) == 0 {
				fmt.Printf("No issues found with label '%s' in %s\n", label, repo)
				return nil
			}

			fmt.Printf("Found %d issue(s). Generating task specs...\n", len(issues))

			tasksDir := resolveTasksDir()
			repoShort := ingest.RepoShortName(repo)

			for _, issue := range issues {
				taskID := ingest.IssueToTaskID(issue.Title)
				taskFile := filepath.Join(tasksDir, taskID+".yaml")

				content := fmt.Sprintf(`id: %s
title: "%s"
source: "%s#%d"
source_type: issue
description: |
  %s
target_repos:
  - %s
context_hints: []
acceptance_criteria:
  - "Issue %s#%d requirements are satisfied"
  - "All repo checks pass"
pr_labels:
  - "minion"
`, taskID, issue.Title, repo, issue.Number, issue.Body, repoShort, repo, issue.Number)

				if err := os.WriteFile(taskFile, []byte(content), 0o644); err != nil {
					return fmt.Errorf("writing task file: %w", err)
				}
				fmt.Printf("Created: %s\n", taskFile)
			}

			fmt.Printf("\nDone. Task specs written to %s/\n", tasksDir)
			return nil
		},
	}

	cmd.Flags().StringVar(&label, "label", "minion-ready", "issue label to filter by")
	return cmd
}

func resolveTasksDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "tasks"
	}
	return filepath.Join(wd, "tasks")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
