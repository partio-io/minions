package checks

import (
	"fmt"
	"os/exec"
)

func runGo(repoPath string) (string, error) {
	// make lint
	out, err := runMake(repoPath, "lint")
	if err != nil {
		return out, fmt.Errorf("make lint failed: %w", err)
	}

	// make test
	testOut, err := runMake(repoPath, "test")
	out += "\n" + testOut
	if err != nil {
		return out, fmt.Errorf("make test failed: %w", err)
	}

	return out, nil
}

func runMake(dir, target string) (string, error) {
	cmd := exec.Command("make", target)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
