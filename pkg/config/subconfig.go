package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// SubConfig is the schema for per-directory gorgon.yml override files.
// Only a safe subset of Config fields are overrideable at directory level.
type SubConfig struct {
	// Replace semantics — deepest match wins
	Operators  []string  `yaml:"operators,omitempty"`
	Threshold  *float64  `yaml:"threshold,omitempty"` // pointer: nil means "not set"
	Tests      []string  `yaml:"tests,omitempty"`
	Concurrent string    `yaml:"concurrent,omitempty"`

	// Merge semantics — all levels in the chain contribute
	Exclude    []string        `yaml:"exclude,omitempty"`
	Include    []string        `yaml:"include,omitempty"`
	Skip       []string        `yaml:"skip,omitempty"`
	SkipFunc   []string        `yaml:"skip_func,omitempty"`
	Suppress   []SuppressEntry `yaml:"suppress,omitempty"`
	DirRules   []DirOperatorRule `yaml:"dir_rules,omitempty"`
}

func LoadSubConfig(path string) (*SubConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read sub-config %s: %w", path, err)
	}
	var sc SubConfig
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("failed to parse sub-config %s: %w", path, err)
	}
	return &sc, nil
}
