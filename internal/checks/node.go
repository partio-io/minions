package checks

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

func runNode(repoPath string) (string, error) {
	var allOutput string

	// Install deps if node_modules missing
	if !fileExists(filepath.Join(repoPath, "node_modules")) {
		out, _ := runNPM(repoPath, "ci", "--ignore-scripts")
		allOutput += out + "\n"
	}

	// npm run lint
	out, err := runNPM(repoPath, "run", "lint")
	allOutput += out + "\n"
	if err != nil {
		return allOutput, fmt.Errorf("npm run lint failed: %w", err)
	}

	// npm run build
	out, err = runNPM(repoPath, "run", "build")
	allOutput += out
	if err != nil {
		return allOutput, fmt.Errorf("npm run build failed: %w", err)
	}

	return allOutput, nil
}

func runNPM(dir string, args ...string) (string, error) {
	cmd := exec.Command("npm", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
