# CLAUDE.md — partio-io/minions

## Project Overview

Minions is a lightweight system for orchestrating unattended Claude Code agents that generate PRs across all Partio repos. Inspired by Stripe's minions (1000+ PRs/week), adapted for Partio's multi-repo setup.

**Core principle:** One-shot execution — no human interaction mid-run. Each minion gets a task spec, runs in an isolated git worktree, and produces a PR.

**Module:** `github.com/partio-io/minions`
**Go version:** 1.25.0
**CLI framework:** Cobra
**Dependencies:** cobra + yaml.v3 (2 direct deps)

## How It Works

1. **Task specs** (`tasks/*.yaml`) define what to build — target repos, acceptance criteria, context hints
2. **Ingestor** (`minions ingest`) generates task specs from external sources (changelogs, blogs, issues)
3. **Proposer** (`minions propose`) daily changelog monitoring → proposal issues with embedded task YAML
4. **Orchestrator** (`minions run`) executes tasks: worktree → prompt → `claude -p` → lint/test → PR
5. **Doc minion** (`minions doc`) generates documentation PRs for existing PRs

## File Structure

```
cmd/minions/              CLI commands (run, ingest, propose, doc, version)
internal/
  task/                   Task struct, YAML loading, validation
  prompt/
    templates/            Embedded prompt templates (prompt.md, doc-prompt.md, ingest-prompt.md)
    embed.go              //go:embed for templates
    build.go              Task prompt construction
    doc.go                Doc prompt construction
  worktree/               Git worktree create/cleanup helpers
  checks/                 Deterministic checks per repo type (go/node/docs)
  claude/                 claude -p headless execution wrapper
  pr/                     PR creation + cross-linking
  ingest/                 Source ingestion (changelog, blog, issues, task generation)
  propose/                Proposal pipeline (sources, version detection, issue creation)
  git/                    Git command helpers
  config/                 Env-based configuration
  log/                    slog setup
sources.yaml              Monitored changelog sources + last-processed version state
tasks/
  examples/               Example task specs
templates/
  task.yaml               Task spec schema/reference
```

## Build & Test

```bash
make build       # Compile binary (embeds version from git tags)
make test        # Run all tests
make lint        # Run golangci-lint
make install     # Build and install to $GOPATH/bin
make clean       # Remove compiled binary
```

## Multi-Repo Workspace

All 5 Partio repos are checked out side-by-side (locally in Tactic workspaces, in Actions via multi-checkout):

```
workspace/
  cli/       # Go CLI — `make lint && make test`
  app/       # Next.js dashboard — `npm run lint && npm run build`
  docs/      # Mintlify docs — `mintlify build` (if available)
  site/      # Next.js marketing site — `npm run lint && npm run build`
  extension/ # Browser extension
  minions/   # This repo
```

The workspace root is always one directory up from `minions/`.

## Key Design Decisions

- **Go binary** — self-contained, no shell script dependencies (yq, python3 removed)
- **Embedded templates** — prompt templates compiled into the binary via `//go:embed`
- **Git worktrees for isolation** — no devboxes needed, worktrees are cheap
- **CLAUDE.md as context** — each repo's CLAUDE.md is automatically included in minion prompts
- **`claude -p` headless mode** — one-shot execution, no interactive prompts
- **Deterministic checks sandwich agent work** — lint/test before PR, retry once on failure

## Commands

```bash
# Run a single task
minions run tasks/example.yaml

# Run all pending tasks
minions run tasks/

# Dry run (prompt generation only)
minions run tasks/example.yaml --dry-run

# Parallel execution
minions run tasks/ --parallel 3

# Ingest features from changelog
minions ingest changelog <url> [version]

# Ingest from blog post
minions ingest blog <url>

# Ingest from GitHub issues
minions ingest issues <repo> [--label <label>]

# Propose features from monitored changelogs
minions propose

# Propose dry run (show what issues would be created)
minions propose --dry-run

# Propose from a specific source only
minions propose --source entireio-cli

# Generate doc PR for an existing PR
minions doc --pr <repo>#<number>

# Print version
minions version
```

## Environment Variables

| Variable | Required | Purpose |
|----------|----------|---------|
| `ANTHROPIC_API_KEY` | Yes | Claude API access |
| `GH_TOKEN` | Yes | GitHub API + PR creation |
| `WORKSPACE_ROOT` | No | Override workspace root (default: `../` from minions/) |
| `MINION_MAX_TURNS` | No | Max Claude turns (default: 30) |
| `MINION_DRY_RUN` | No | Set to `1` to skip execution |
| `MINION_LOG_LEVEL` | No | Log level (debug, info, warn, error) |

## Conventions

- Task YAML files use kebab-case filenames matching the task `id`
- Branch names: `minion/<task-id>`
- PR labels always include `minion`
- One primary concern per file (matching cli/ repo patterns)
- `slog` for structured logging to stderr
- `exec.Command` for external tools (git, gh, claude, make, npm)
