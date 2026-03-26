package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var (
		org   string
		repos string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new minions project",
		Long:  `Generates .minions/project.yaml and GitHub Actions workflow templates for a new minion project.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if org == "" {
				return fmt.Errorf("--org is required")
			}
			if repos == "" {
				return fmt.Errorf("--repos is required")
			}

			repoList := strings.Split(repos, ",")
			for i, r := range repoList {
				repoList[i] = strings.TrimSpace(r)
			}

			// Determine output directory
			outDir := "."
			if len(args) > 0 {
				outDir = args[0]
			}

			minionsDir := filepath.Join(outDir, ".minions")
			if err := os.MkdirAll(minionsDir, 0755); err != nil {
				return fmt.Errorf("creating .minions directory: %w", err)
			}

			// Generate project.yaml
			projectPath := filepath.Join(minionsDir, "project.yaml")
			if err := generateProjectYAML(projectPath, org, repoList); err != nil {
				return err
			}
			fmt.Printf("Created %s\n", projectPath)

			// Generate GitHub Actions workflow
			workflowDir := filepath.Join(outDir, ".github", "workflows")
			if err := os.MkdirAll(workflowDir, 0755); err != nil {
				return fmt.Errorf("creating workflows directory: %w", err)
			}

			workflowPath := filepath.Join(workflowDir, "minion.yml")
			if err := generateWorkflow(workflowPath, org); err != nil {
				return err
			}
			fmt.Printf("Created %s\n", workflowPath)

			fmt.Println("\nDone! Review the generated files and customize as needed.")
			fmt.Println("Add your ANTHROPIC_API_KEY and GH_TOKEN as GitHub Actions secrets.")
			return nil
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "GitHub organization or owner (required)")
	cmd.Flags().StringVar(&repos, "repos", "", "comma-separated list of target repo names (required)")

	return cmd
}

func generateProjectYAML(path, org string, repos []string) error {
	var b strings.Builder
	b.WriteString("version: \"1\"\n\n")
	fmt.Fprintf(&b, "org: %s\n\n", org)

	b.WriteString("# Principal repository — where proposals and status tracking happen\n")
	b.WriteString("principal:\n")
	b.WriteString("  name: minions\n")
	fmt.Fprintf(&b, "  full_name: %s/minions\n\n", org)

	b.WriteString("# Target repositories\n")
	b.WriteString("repos:\n")
	for _, repo := range repos {
		fmt.Fprintf(&b, "  - name: %s\n", repo)
		fmt.Fprintf(&b, "    full_name: %s/%s\n", org, repo)
		fmt.Fprintf(&b, "    build_info: \"TODO: describe build/test commands\"\n\n")
	}

	b.WriteString("# Credentials — env var NAMES, not values\n")
	b.WriteString("credentials:\n")
	b.WriteString("  anthropic_api_key_env: ANTHROPIC_API_KEY\n")
	b.WriteString("  gh_token_env: GH_TOKEN\n\n")

	b.WriteString("# Defaults\n")
	b.WriteString("defaults:\n")
	b.WriteString("  max_turns: 30\n")
	b.WriteString("  pr_labels:\n")
	b.WriteString("    - minion\n")

	return os.WriteFile(path, []byte(b.String()), 0644)
}

func generateWorkflow(path, _ string) error {
	const workflow = `name: Minion

on:
  workflow_dispatch:
    inputs:
      task_file:
        description: "Path to task YAML file"
        required: true

jobs:
  run:
    runs-on: ubuntu-latest
    steps:
      - name: Install yq
        run: |
          sudo wget -qO /usr/local/bin/yq https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64
          sudo chmod +x /usr/local/bin/yq

      - name: Checkout principal repo
        uses: actions/checkout@v4
        with:
          path: minions

      - name: Checkout project repos
        env:
          GH_TOKEN: ${{ secrets.GH_TOKEN }}
        run: |
          for ENTRY in $(yq -r '.repos[] | .full_name + ":" + .name' minions/.minions/project.yaml); do
            FULL="${ENTRY%%:*}"
            SHORT="${ENTRY##*:}"
            gh repo clone "$FULL" "$SHORT" -- --depth=1
          done

      - name: Configure git auth
        env:
          GH_TOKEN: ${{ secrets.GH_TOKEN }}
        run: git config --global url."https://x-access-token:${GH_TOKEN}@github.com/".insteadOf "https://github.com/"

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version-file: minions/go.mod

      - name: Build minions
        working-directory: minions
        run: go build -o ../minions-bin ./cmd/minions

      - name: Run task
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          GH_TOKEN: ${{ secrets.GH_TOKEN }}
          WORKSPACE_ROOT: ${{ github.workspace }}
        run: ./minions-bin run ${{ inputs.task_file }}
`

	return os.WriteFile(path, []byte(workflow), 0644)
}
