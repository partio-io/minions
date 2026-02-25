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
