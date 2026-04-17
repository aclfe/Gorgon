package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type SuppressEntry struct {
	Location  string   `yaml:"location"`
	Operators []string `yaml:"operators,omitempty"`
}

type DirOperatorRule struct {
	Dir       string   `yaml:"dir"`
	Whitelist []string `yaml:"whitelist,omitempty"`
	Blacklist []string `yaml:"blacklist,omitempty"`
}

type Config struct {
	Operators    []string          `yaml:"operators"`
	Concurrent   string            `yaml:"concurrent"`
	Threshold    float64           `yaml:"threshold"`
	Cache        bool              `yaml:"cache"`
	DryRun       bool              `yaml:"dry_run"`
	Debug        bool              `yaml:"debug"`
	ProgBar      bool              `yaml:"progbar"`
	ShowKilled   bool              `yaml:"show_killed"`
	ShowSurvived bool              `yaml:"show_survived"`
	Format       string            `yaml:"format"`
	Output       string            `yaml:"output"`
	CPUProfile   string            `yaml:"cpu_profile"`
	Exclude      []string          `yaml:"exclude"`
	Include      []string          `yaml:"include"`
	Skip         []string          `yaml:"skip"`
	SkipFunc     []string          `yaml:"skip_func"`
	Tests        []string          `yaml:"tests"`
	Base         string            `yaml:"base"`
	Suppress     []SuppressEntry   `yaml:"suppress"`
	Diff         string            `yaml:"diff,omitempty"`
	DirRules     []DirOperatorRule `yaml:"dir_rules,omitempty"`
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
		DirRules:   []DirOperatorRule{},
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

func (c *Config) AddSuppression(location string, operators []string) {
	location = strings.TrimSpace(location)
	if location == "" {
		return
	}

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
