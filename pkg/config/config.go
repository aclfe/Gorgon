package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Mutations  MutationsConfig  `yaml:"mutations"`
	Testing    TestingConfig    `yaml:"testing"`
	Engine     EngineConfig     `yaml:"engine"`
	Output     OutputConfig     `yaml:"output"`
}

type MutationsConfig struct {
	Operators []string `yaml:"operators"`
	Exclude   []string `yaml:"exclude"`
}

type TestingConfig struct {
	Timeout     time.Duration `yaml:"timeout"`
	Concurrent  int           `yaml:"concurrent"`
	FailFast    bool          `yaml:"fail_fast"`
}

type EngineConfig struct {
	PrintAST    bool     `yaml:"print_ast"`
	Parallel    int      `yaml:"parallel"`
}

type OutputConfig struct {
	Format      string   `yaml:"format"`
	ShowSkipped bool     `yaml:"show_skipped"`
}

func Default() *Config {
	return &Config{
		Mutations: MutationsConfig{
			Operators: []string{"all"},
			Exclude:   []string{},
		},
		Testing: TestingConfig{
			Timeout:    10 * time.Second,
			Concurrent: 0,
			FailFast:   false,
		},
		Engine: EngineConfig{
			PrintAST: false,
			Parallel: 0,
		},
		Output: OutputConfig{
			Format:      "text",
			ShowSkipped: false,
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
	if c.Testing.Timeout <= 0 {
		c.Testing.Timeout = 10 * time.Second
	}

	if c.Testing.Concurrent <= 0 {
		c.Testing.Concurrent = 0
	}

	validFormats := map[string]bool{"text": true, "json": true, "html": true}
	if !validFormats[c.Output.Format] {
		c.Output.Format = "text"
	}

	return nil
}
