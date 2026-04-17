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

type ExternalSuite struct {
	Name         string   `yaml:"name"`
	Paths        []string `yaml:"paths"`
	Tags         []string `yaml:"tags,omitempty"`
	ShortCircuit bool     `yaml:"short_circuit"`
	RunMode      string   `yaml:"run_mode,omitempty"`
}

type ExternalSuitesConfig struct {
	Enabled bool             `yaml:"enabled"`
	RunMode string           `yaml:"run_mode"`
	Suites  []ExternalSuite  `yaml:"suites"`
}

type Config struct {
	Operators         []string          `yaml:"operators"`
	Concurrent        string            `yaml:"concurrent"`
	Threshold         float64           `yaml:"threshold"`
	Cache             bool              `yaml:"cache"`
	DryRun            bool              `yaml:"dry_run"`
	Debug             bool              `yaml:"debug"`
	ProgBar           bool              `yaml:"progbar"`
	ShowKilled        bool              `yaml:"show_killed"`
	ShowSurvived      bool              `yaml:"show_survived"`
	Format            string            `yaml:"format"`
	Output            string            `yaml:"output"`
	CPUProfile        string            `yaml:"cpu_profile"`
	Exclude           []string          `yaml:"exclude"`
	Include           []string          `yaml:"include"`
	Skip              []string          `yaml:"skip"`
	SkipFunc          []string          `yaml:"skip_func"`
	Tests             []string          `yaml:"tests"`
	Base              string            `yaml:"base"`
	Suppress          []SuppressEntry   `yaml:"suppress"`
	Diff              string            `yaml:"diff,omitempty"`
	DirRules          []DirOperatorRule `yaml:"dir_rules,omitempty"`
	UnitTestsEnabled  bool              `yaml:"unit_tests_enabled"`
	ExternalSuites    ExternalSuitesConfig `yaml:"external_suites"`
}

func Default() *Config {
	return &Config{
		Operators:        []string{"all"},
		Concurrent:       "all",
		Threshold:        0,
		Cache:            false,
		DryRun:           false,
		Exclude:          []string{},
		Include:          []string{},
		Skip:             []string{},
		SkipFunc:         []string{},
		Tests:            []string{},
		Suppress:         []SuppressEntry{},
		DirRules:         []DirOperatorRule{},
		UnitTestsEnabled: true,
		ExternalSuites: ExternalSuitesConfig{
			Enabled: false,
			RunMode: "after_unit",
			Suites:  []ExternalSuite{},
		},
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
	// Create organized YAML with comments and proper structure
	var lines []string
	
	lines = append(lines, "# === Core Mutation Settings ===")
	lines = append(lines, fmt.Sprintf("operators:"))
	for _, op := range c.Operators {
		lines = append(lines, fmt.Sprintf("    - %s", op))
	}
	lines = append(lines, fmt.Sprintf("threshold: %.0f", c.Threshold))
	lines = append(lines, "")
	
	lines = append(lines, "# === Execution Settings ===")
	lines = append(lines, fmt.Sprintf("concurrent: %s", c.Concurrent))
	lines = append(lines, fmt.Sprintf("cache: %t", c.Cache))
	lines = append(lines, fmt.Sprintf("dry_run: %t", c.DryRun))
	lines = append(lines, fmt.Sprintf("progbar: %t", c.ProgBar))
	lines = append(lines, "")
	
	lines = append(lines, "# === Test Configuration ===")
	lines = append(lines, fmt.Sprintf("unit_tests_enabled: %t", c.UnitTestsEnabled))
	lines = append(lines, "tests:")
	if len(c.Tests) == 0 {
		lines = append(lines, "    []")
	} else {
		for _, test := range c.Tests {
			lines = append(lines, fmt.Sprintf("    - %s", test))
		}
	}
	lines = append(lines, "")
	
	lines = append(lines, "# === External Test Suites ===")
	lines = append(lines, "external_suites:")
	lines = append(lines, fmt.Sprintf("    enabled: %t", c.ExternalSuites.Enabled))
	lines = append(lines, fmt.Sprintf("    run_mode: %s", c.ExternalSuites.RunMode))
	lines = append(lines, "    suites:")
	if len(c.ExternalSuites.Suites) == 0 {
		lines = append(lines, "        []")
	} else {
		for _, suite := range c.ExternalSuites.Suites {
			lines = append(lines, fmt.Sprintf("        - name: %s", suite.Name))
			lines = append(lines, "          paths:")
			for _, path := range suite.Paths {
				lines = append(lines, fmt.Sprintf("            - %s", path))
			}
			if len(suite.Tags) > 0 {
				lines = append(lines, "          tags:")
				for _, tag := range suite.Tags {
					lines = append(lines, fmt.Sprintf("            - %s", tag))
				}
			}
			lines = append(lines, fmt.Sprintf("          short_circuit: %t", suite.ShortCircuit))
		}
	}
	lines = append(lines, "")
	
	lines = append(lines, "# === File Filtering ===")
	lines = append(lines, "exclude:")
	if len(c.Exclude) == 0 {
		lines = append(lines, "    []")
	} else {
		for _, ex := range c.Exclude {
			lines = append(lines, fmt.Sprintf("    - '%s'", ex))
		}
	}
	lines = append(lines, "include:")
	if len(c.Include) == 0 {
		lines = append(lines, "    []")
	} else {
		for _, inc := range c.Include {
			lines = append(lines, fmt.Sprintf("    - %s", inc))
		}
	}
	lines = append(lines, "skip:")
	if len(c.Skip) == 0 {
		lines = append(lines, "    []")
	} else {
		for _, skip := range c.Skip {
			lines = append(lines, fmt.Sprintf("    - %s", skip))
		}
	}
	lines = append(lines, "skip_func:")
	if len(c.SkipFunc) == 0 {
		lines = append(lines, "    []")
	} else {
		for _, sf := range c.SkipFunc {
			lines = append(lines, fmt.Sprintf("    - %s", sf))
		}
	}
	lines = append(lines, "")
	
	lines = append(lines, "# === Advanced Options ===")
	lines = append(lines, fmt.Sprintf("diff: \"%s\"", c.Diff))
	lines = append(lines, fmt.Sprintf("base: \"%s\"", c.Base))
	lines = append(lines, fmt.Sprintf("debug: %t", c.Debug))
	lines = append(lines, "")
	
	lines = append(lines, "# === Output Settings ===")
	lines = append(lines, fmt.Sprintf("show_killed: %t", c.ShowKilled))
	lines = append(lines, fmt.Sprintf("show_survived: %t", c.ShowSurvived))
	lines = append(lines, fmt.Sprintf("format: %s", c.Format))
	lines = append(lines, fmt.Sprintf("output: \"%s\"", c.Output))
	lines = append(lines, fmt.Sprintf("cpu_profile: \"%s\"", c.CPUProfile))
	lines = append(lines, "")
	
	if len(c.DirRules) > 0 {
		lines = append(lines, "# === Directory Rules ===")
		lines = append(lines, "dir_rules:")
		for _, rule := range c.DirRules {
			lines = append(lines, fmt.Sprintf("    - dir: %s", rule.Dir))
			if len(rule.Whitelist) > 0 {
				lines = append(lines, "      whitelist:")
				for _, w := range rule.Whitelist {
					lines = append(lines, fmt.Sprintf("        - %s", w))
				}
			}
			if len(rule.Blacklist) > 0 {
				lines = append(lines, "      blacklist:")
				for _, b := range rule.Blacklist {
					lines = append(lines, fmt.Sprintf("        - %s", b))
				}
			}
		}
		lines = append(lines, "")
	}
	
	// Suppressions (Auto-managed)
	lines = append(lines, "# === Suppressions (Auto-managed) ===")
	lines = append(lines, "suppress:")
	if len(c.Suppress) == 0 {
		lines = append(lines, "    []")
	} else {
		for _, sup := range c.Suppress {
			lines = append(lines, fmt.Sprintf("    - location: %s", sup.Location))
			if len(sup.Operators) > 0 {
				lines = append(lines, "      operators:")
				for _, op := range sup.Operators {
					lines = append(lines, fmt.Sprintf("        - %s", op))
				}
			}
		}
	}
	
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}
