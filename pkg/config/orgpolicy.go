package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const OrgPolicyFilename = "gorgon-org.yml"

// All fields are optional. Absent fields impose no constraint.
type OrgPolicy struct {
	// ThresholdFloor is the minimum mutation score any package is allowed
	// to report. Any threshold below this value is silently raised to meet it.
	ThresholdFloor float64 `yaml:"threshold_floor,omitempty"`

	// RequiredOperators lists operators that must be active for every target,
	// regardless of what operators a root config or sub-config specifies.
	RequiredOperators []string `yaml:"required_operators,omitempty"`

	// ForbiddenOperators lists operators that must never run.
	ForbiddenOperators []string `yaml:"forbidden_operators,omitempty"`

	// LockedSettings prevents any config below this level from changing
	// specific settings. Valid values: "skip", "skip_func", "exclude",
	// "include", "tests", "cache", "concurrent", "operators", "threshold",
	// "sub_config_mode", "violation_mode".
	LockedSettings []string `yaml:"locked_settings,omitempty"`

	// ForcedSkipPaths are paths that are always skipped, appended after
	// all other resolution. Useful for generated code.
	ForcedSkipPaths []string `yaml:"forced_skip_paths,omitempty"`

	// ForcedExcludePatterns are glob patterns that are always excluded.
	ForcedExcludePatterns []string `yaml:"forced_exclude_patterns,omitempty"`

	// MinConcurrent sets a floor on the concurrent setting.
	MinConcurrent int `yaml:"min_concurrent,omitempty"`

	// RequireCache forces cache: true across all configs when set.
	RequireCache *bool `yaml:"require_cache,omitempty"`
}

// LoadOrgPolicy reads gorgon-org.yml from path.
// If the file does not exist, it returns a zero-value policy and no error.
func LoadOrgPolicy(path string) (*OrgPolicy, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &OrgPolicy{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read org policy %s: %w", path, err)
	}
	var p OrgPolicy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("failed to parse org policy %s: %w", path, err)
	}
	return &p, nil
}

// IsZero reports whether the policy imposes any constraints at all.
func (p *OrgPolicy) IsZero() bool {
	return p.ThresholdFloor == 0 &&
		len(p.RequiredOperators) == 0 &&
		len(p.ForbiddenOperators) == 0 &&
		len(p.LockedSettings) == 0 &&
		len(p.ForcedSkipPaths) == 0 &&
		len(p.ForcedExcludePatterns) == 0 &&
		p.MinConcurrent == 0 &&
		p.RequireCache == nil
}
