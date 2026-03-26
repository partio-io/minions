package task

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// LoadFile loads a single task from a YAML file.
func LoadFile(path string) (*Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading task file %s: %w", path, err)
	}

	var t Task
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parsing task file %s: %w", path, err)
	}

	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("validating task file %s: %w", path, err)
	}

	return &t, nil
}

// LoadRepoTasks loads task YAML files from a repo's .minions/tasks/ directory.
// Returns nil (not an error) if the directory doesn't exist.
func LoadRepoTasks(repoPath string) ([]*Task, error) {
	tasksDir := filepath.Join(repoPath, ".minions", "tasks")
	if _, err := os.Stat(tasksDir); os.IsNotExist(err) {
		return nil, nil
	}
	return LoadDir(tasksDir)
}

// DiscoverAll aggregates tasks from .minions/tasks/ across multiple repos.
func DiscoverAll(workspaceRoot string, repos []string) ([]*Task, error) {
	var all []*Task
	for _, repo := range repos {
		repoPath := filepath.Join(workspaceRoot, repo)
		tasks, err := LoadRepoTasks(repoPath)
		if err != nil {
			return nil, fmt.Errorf("loading tasks from %s: %w", repo, err)
		}
		// Default target_repos to the repo itself if not specified
		for _, t := range tasks {
			if len(t.TargetRepos) == 0 {
				t.TargetRepos = []string{repo}
			}
		}
		all = append(all, tasks...)
	}
	return all, nil
}

// LoadDir loads all task YAML files from a directory, excluding the examples/ subdirectory.
func LoadDir(dir string) ([]*Task, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip examples/ subdirectory
		if info.IsDir() && info.Name() == "examples" {
			return filepath.SkipDir
		}
		if !info.IsDir() && filepath.Ext(path) == ".yaml" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking task directory %s: %w", dir, err)
	}

	sort.Strings(files)

	var tasks []*Task
	for _, f := range files {
		t, err := LoadFile(f)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}

	return tasks, nil
}
