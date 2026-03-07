package propose

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Source represents a monitored changelog source.
type Source struct {
	Name        string `yaml:"name"`
	URL         string `yaml:"url"`
	Type        string `yaml:"type"`
	Repo        string `yaml:"repo,omitempty"`
	LastVersion string `yaml:"last_version"`
}

// SourcesConfig is the top-level structure of sources.yaml.
type SourcesConfig struct {
	Sources []Source `yaml:"sources"`
}

// LoadSources reads and parses a sources.yaml file.
func LoadSources(path string) (*SourcesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading sources file: %w", err)
	}

	var cfg SourcesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing sources file: %w", err)
	}

	return &cfg, nil
}

// SaveSources writes the sources config back to disk.
func SaveSources(path string, cfg *SourcesConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling sources: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing sources file: %w", err)
	}

	return nil
}
