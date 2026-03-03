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

Changelog Monitor  →  Proposal Issues  →  Auto-Approve (24h)  →  Execution  →  Done/Failed
(minions propose)     (minion-proposal)   (minions approve)      (minions run)  (labels + close)
         ↑ cron 08:00,20:00 UTC            ↑ cron 09:00,21:00 UTC
                                            Human can veto with 'do-not-build' label
                                            Human can fast-track with '/minion build' comment
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

### Propose Features (`minions propose`)

Monitor changelogs for new releases and create proposal issues:

```bash
minions propose                        # Check all sources, create issues
minions propose --dry-run              # Preview without creating issues
minions propose --source entireio-cli  # Check a specific source only
```

Proposal issues include embedded task YAML. They are auto-approved after 24h if no one adds the `do-not-build` label. Comment `/minion build` to fast-track.

### Approve Proposals (`minions approve`)

Auto-approve proposals that have passed the review window:

```bash
minions approve                    # Approve proposals older than 24h
minions approve --delay 0h         # Approve all eligible now
minions approve --dry-run           # Preview without making changes
```

### Doc Minion (`minions doc`)

Generate documentation PRs for existing code PRs:

```bash
minions doc --pr partio-io/cli#42
minions doc --pr partio-io/app#15 --dry-run
```

## GitHub Actions

Four workflows for CI/CD execution:

- **`minion.yml`** — Full minion pipeline (manual trigger, issue label, or `/minion build` comment)
- **`propose.yml`** — Twice-daily changelog monitoring + proposal issue creation (08:00, 20:00 UTC)
- **`approve.yml`** — Auto-approve proposals after 24h review window (09:00, 21:00 UTC)
- **`doc-minion.yml`** — Documentation updates (manual trigger)

## Requirements

- Go 1.25+ (to build the binary)
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) (`npm install -g @anthropic-ai/claude-code`)
- [GitHub CLI](https://cli.github.com/) (`gh`)
- Node.js 22+ (for app/site repo checks)
