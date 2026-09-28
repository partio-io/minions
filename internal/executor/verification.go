package executor

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/partio-io/minions/internal/program"
	"github.com/partio-io/minions/internal/sliceguard"
)

// verifier reports whether the work an agent produced is acceptable. When it
// is not, the verifier also returns the failure text that scopes the fix-it
// session.
type verifier func() (bool, string)

// verification is one run of the check-and-retry path: the verifiers to run,
// and the labels that name the run in printed output, in logs and in debug
// artifacts. A second kind of verification joins the list, so the retry
// helper keeps its signature.
type verification struct {
	scope     string
	debugBase string
	logAttrs  []any
	verifiers []verifier
}

// run runs every verifier and combines the results. The run passes only when
// every verifier passes, and the failure text holds the output of each
// verifier that failed. Each output ends on its own line, so one verifier's
// last line never runs into the next verifier's first line.
func (v verification) run() (bool, string) {
	allPass := true
	var failed strings.Builder
	for _, verify := range v.verifiers {
		pass, output := verify()
		if pass {
			continue
		}
		allPass = false
		if output = strings.TrimSpace(output); output == "" {
			continue
		}
		failed.WriteString(output)
		failed.WriteByte('\n')
	}
	return allPass, failed.String()
}

// agentVerification verifies an agent's worktrees with the deterministic
// checks, and labels the run with the agent.
func agentVerification(agent *program.AgentDef, worktreePaths []string) verification {
	return verification{
		scope:     "agent " + agent.Name,
		debugBase: "agent-" + agent.Name,
		logAttrs:  []any{"agent", agent.Name},
		verifiers: []verifier{checksVerifier(worktreePaths)},
	}
}

// sliceVerification verifies one slice's worktrees with the deterministic
// checks and the slice boundary guard, and labels the run with the slice
// being built. worktreeRepos names the repository of each worktree, and
// branchName is the run's branch, which the guard reads for the earlier
// slices' work.
func sliceVerification(agent *program.AgentDef, worktreePaths, worktreeRepos []string, branchName string, num, total int) verification {
	return verification{
		scope:     fmt.Sprintf("slice %d/%d", num, total),
		debugBase: fmt.Sprintf("agent-%s-slice-%d", agent.Name, num),
		logAttrs:  []any{"agent", agent.Name, "slice", num},
		verifiers: []verifier{
			checksVerifier(worktreePaths),
			guardVerifier(worktreePaths, worktreeRepos, branchName, num),
		},
	}
}

// guardVerifier runs the slice boundary guard over the worktrees: it fails
// when an earlier slice's contribution has no reference left in the working
// tree, and its failure text is what scopes the fix session. Slice one has
// no earlier slice and always passes.
func guardVerifier(worktreePaths, worktreeRepos []string, branchName string, num int) verifier {
	return func() (bool, string) {
		abandoned := abandonedContributions(worktreePaths, worktreeRepos, branchName, num)
		if len(abandoned) == 0 {
			return true, ""
		}
		for _, c := range abandoned {
			slog.Warn("slice boundary guard: earlier contribution abandoned", "identifier", c.Identifier, "file", c.File, "added_by_slice", c.Slice, "slice", num)
		}
		return false, sliceguard.FailureText(abandoned)
	}
}

// checksVerifier runs the deterministic checks over the worktrees.
func checksVerifier(worktreePaths []string) verifier {
	return func() (bool, string) { return runChecks(worktreePaths) }
}
