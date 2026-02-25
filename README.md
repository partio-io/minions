# Partio Minions

One-shot coding agents that generate PRs across all Partio repos.

Inspired by [Stripe's minions system](https://www.youtube.com/watch?v=example) — fully unattended agents that create, test, and submit PRs without human interaction during execution.

## Quick Start

```bash
# 1. Build the binary
make build

# 2. Define a task
cat tasks/examples/detect-external-hooks.yaml

# 3. Dry run to preview the generated prompt
./minions run tasks/examples/detect-external-hooks.yaml --dry-run

# 4. Execute the minion
export ANTHROPIC_API_KEY="sk-ant-..."
export GH_TOKEN="ghp_..."
./minions run tasks/examples/detect-external-hooks.yaml
```

## Architecture

```
Source Ingestor  →  Task Specs (YAML)  →  Orchestrator  →  Minions  →  PRs
(minions ingest)                         (minions run)    (worktrees)  (gh pr create)
```

Each minion:
1. Creates a git worktree for isolation
2. Receives a pre-hydrated prompt (task spec + CLAUDE.md + context hints)
3. Runs `claude -p` in headless mode
4. Passes deterministic checks (lint, test, build)
5. Creates a PR with proper labels and cross-links

## Commands

### Run Tasks (`minions run`)

Execute task specs end-to-end:

```bash
minions run tasks/my-task.yaml              # Single task
minions run tasks/                          # All pending tasks
minions run tasks/ --parallel 3             # Parallel execution
minions run tasks/ --dry-run                # Preview without executing
```

### Ingest Sources (`minions ingest`)

Generate task specs from external sources:

```bash
minions ingest changelog <url> [version]    # Parse changelog for feature ideas
minions ingest blog <url>                   # Extract ideas from blog posts
minions ingest issues <repo> [--label lbl]  # Convert labeled issues to tasks
```

### Doc Minion (`minions doc`)

Generate documentation PRs for existing code PRs:

```bash
minions doc --pr partio-io/cli#42
minions doc --pr partio-io/app#15 --dry-run
```

## GitHub Actions

Two workflows for CI/CD execution:

- **`minion.yml`** — Full minion pipeline (manual trigger or issue label)
- **`doc-minion.yml`** — Documentation updates (manual trigger)

## Requirements

- Go 1.25+ (to build the binary)
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) (`npm install -g @anthropic-ai/claude-code`)
- [GitHub CLI](https://cli.github.com/) (`gh`)
- Node.js 22+ (for app/site repo checks)
