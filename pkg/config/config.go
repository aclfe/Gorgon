package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// SuppressEntry represents a single suppression target in the config.
// Format in YAML: "path/to/file.go:6" with optional "operators:" sub-key.
type SuppressEntry struct {
	Location  string   `yaml:"location"`  // e.g. "examples/foo.go:6"
	Operators []string `yaml:"operators,omitempty"` // empty = all operators on that line
}

type Config struct {
	Operators  []string        `yaml:"operators"`
	Concurrent string          `yaml:"concurrent"`
	Threshold  float64         `yaml:"threshold"`
	Cache      bool            `yaml:"cache"`
	DryRun     bool            `yaml:"dry_run"`
	Exclude    []string        `yaml:"exclude"`
	Include    []string        `yaml:"include"`
	Skip       []string        `yaml:"skip"`
	SkipFunc   []string        `yaml:"skip_func"`
	Tests      []string        `yaml:"tests"`
	Base       string          `yaml:"base"`
	Debug      bool            `yaml:"debug"`
	Suppress   []SuppressEntry `yaml:"suppress"`
}

func Default() *Config {
	return &Config{
		Operators:  []string{"all"},
		Concurrent: "all",
		Threshold:  0,
		Cache:      false,
		DryRun:     false,
		Exclude:    []string{},
		Include:    []string{},
		Skip:       []string{},
		SkipFunc:   []string{},
		Tests:      []string{},
		Suppress:   []SuppressEntry{},
	}
}

func Load(path string) (*Config, error) {
	if path == "" {
		return Default(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Concurrent == "" {
		c.Concurrent = "all"
	}
	return nil
}

// AddSuppression appends a suppression entry with location (e.g. "path/to/file.go:6")
// and optional operator list. Duplicates are ignored.
func (c *Config) AddSuppression(location string, operators []string) {
	location = strings.TrimSpace(location)
	if location == "" {
		return
	}
	// Normalize operators list
	var normalized []string
	seen := make(map[string]bool)
	for _, op := range operators {
		op = strings.TrimSpace(op)
		if op != "" && !seen[op] {
			seen[op] = true
			normalized = append(normalized, op)
		}
	}

	for _, existing := range c.Suppress {
		if existing.Location == location {
			// Merge operators if location already exists
			existingOps := make(map[string]bool)
			for _, op := range existing.Operators {
				existingOps[op] = true
			}
			for _, op := range normalized {
				if !existingOps[op] {
					existing.Operators = append(existing.Operators, op)
				}
			}
			return
		}
	}
	c.Suppress = append(c.Suppress, SuppressEntry{
		Location:  location,
		Operators: normalized,
	})
}

// Save writes the config back to the given YAML file path.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}
