# CLAUDE.md — partio-io/minions

## Project Overview

Minions is a standalone program runtime for orchestrating unattended Claude Code agents. It executes `.md` program files that define what an agent should build, then creates PRs with the results.

**Core principle:** One-shot execution — no human interaction mid-run. Each minion gets a program, runs in an isolated git worktree, and produces a PR.

**Module:** `github.com/partio-io/minions`
**Go version:** 1.26.0
**CLI framework:** Cobra
**Dependencies:** cobra, claude-agent-sdk-go, yaml.v3

## Commands

```bash
minions run <program.md>                    # Execute a program
minions run <program.md> --issue 120        # Execute with GitHub issue as context
minions run <program.md> --issue org/repo#120  # Full issue reference
minions run <program.md> --dry-run          # Preview without executing
minions init --org <org> --repos <r1,r2>    # Bootstrap a new project
minions version                             # Print version
```

## File Structure

```
cmd/minions/              CLI commands (run, init, version)
internal/
  program/                Program .md parsing (frontmatter + markdown sections)
  executor/               Agent execution (worktree → claude → checks → PR)
  planner/                Optional planning phase for programs with ## Planner
  project/                .minions/project.yaml loading
  repoconfig/             Per-repo .minions/repo.yaml loading
  workspace/              Repo availability and cloning
  claude/                 Claude Agent SDK wrapper
  pr/                     PR creation + cross-linking
  checks/                 Deterministic checks (auto-detect or config-driven)
  context/                Context tracking and reporting
  worktree/               Git worktree create/cleanup
  git/                    Git command helpers
  config/                 Env-based configuration
  log/                    slog setup
```

## How It Works

1. **Parse program** — load `.md` file with YAML frontmatter (id, target_repos, acceptance_criteria) and markdown body (title, description, agents)
2. **Load project config** — read `.minions/project.yaml` from the host project for repo names, credentials, build info
3. **Ensure repos** — clone missing repos into the workspace
4. **Plan** (optional) — if the program has a `## Planner` section, run a read-only Claude session first
5. **Execute agents** — for each agent (or implicit single agent), create a worktree, run Claude, check for changes
6. **Summarize** — run a cheap Claude call on the diff to generate a PR title and description
7. **Create PRs** — push changes and create PRs via `gh`

## Project Config

Minions requires `.minions/project.yaml` in the host project:

```yaml
version: "1"
org: my-org
principal:
  name: my-repo
  full_name: my-org/my-repo
repos:
  - name: my-repo
    full_name: my-org/my-repo
    build_info: "Go project (build: make test)"
credentials:
  gh_token_env: GH_TOKEN
```

## Build & Test

```bash
make build       # Compile binary
make test        # Run all tests
make lint        # Run golangci-lint
make install     # Build and install to $GOPATH/bin
```

## Environment Variables

| Variable | Required | Purpose |
|----------|----------|---------|
| `GH_TOKEN` | Yes | GitHub API + PR creation |
| `WORKSPACE_ROOT` | No | Override workspace root (default: parent of cwd) |
| `MINION_MAX_TURNS` | No | Max Claude turns (default: 30) |
| `MINION_DRY_RUN` | No | Set to `1` to skip execution |
| `MINION_LOG_LEVEL` | No | Log level (debug, info, warn, error) |

## Commit Format

```
Commit main line

## Objective

<what this change does>

## Why

<why this change is needed>

## How

<how it was implemented>
```

## Conventions

- Branch names: `minion/<task-id>`
- PR labels always include `minion`
- One primary concern per file
- `slog` for structured logging to stderr
- `exec.Command` for external tools (git, gh, claude)
