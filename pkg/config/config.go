package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Operators  []string `yaml:"operators"`
	Concurrent string   `yaml:"concurrent"`
	Threshold  float64  `yaml:"threshold"`
	Cache      bool     `yaml:"cache"`
	DryRun     bool     `yaml:"dry_run"`
	Exclude    []string `yaml:"exclude"`
	Include    []string `yaml:"include"`
	Skip       []string `yaml:"skip"`
	SkipFunc   []string `yaml:"skip_func"`
	Tests      []string `yaml:"tests"`
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
