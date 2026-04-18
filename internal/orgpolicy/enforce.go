package orgpolicy

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

// Violation describes a single policy breach.
type Violation struct {
	Setting  string
	Was      string
	Enforced string
	Reason   string
}

func (v Violation) Error() string {
	return fmt.Sprintf("org policy: %s was %q, enforced to %q (%s)",
		v.Setting, v.Was, v.Enforced, v.Reason)
}

// Result carries the adjusted config and violation log.
type Result struct {
	Config     *config.Config
	Violations []Violation
}

// Apply enforces policy p on cfg and returns an adjusted copy.
func Apply(cfg *config.Config, p *config.OrgPolicy, allOps []mutator.Operator) Result {
	if p == nil || p.IsZero() {
		return Result{Config: cfg}
	}

	out := *cfg
	var violations []Violation

	// Threshold floor
	if p.ThresholdFloor > 0 && out.Threshold < p.ThresholdFloor {
		violations = append(violations, Violation{
			Setting:  "threshold",
			Was:      fmt.Sprintf("%.2f", out.Threshold),
			Enforced: fmt.Sprintf("%.2f", p.ThresholdFloor),
			Reason:   "below org threshold_floor",
		})
		out.Threshold = p.ThresholdFloor
	}

	// Required operators
	if len(p.RequiredOperators) > 0 {
		out.Operators, violations = enforceRequiredOperators(
			out.Operators, p.RequiredOperators, allOps, violations,
		)
	}

	// Forbidden operators
	if len(p.ForbiddenOperators) > 0 {
		out.Operators, violations = enforceForbiddenOperators(
			out.Operators, p.ForbiddenOperators, violations,
		)
	}

	// Forced skip paths
	if len(p.ForcedSkipPaths) > 0 {
		out.Skip = mergeUnique(out.Skip, p.ForcedSkipPaths)
	}

	// Forced exclude patterns
	if len(p.ForcedExcludePatterns) > 0 {
		out.Exclude = mergeUnique(out.Exclude, p.ForcedExcludePatterns)
	}

	// Require cache
	if p.RequireCache != nil && *p.RequireCache && !out.Cache {
		violations = append(violations, Violation{
			Setting:  "cache",
			Was:      "false",
			Enforced: "true",
			Reason:   "require_cache set in org policy",
		})
		out.Cache = true
	}

	// Min concurrent
	if p.MinConcurrent > 0 {
		out.Concurrent, violations = enforceMinConcurrent(
			out.Concurrent, p.MinConcurrent, violations,
		)
	}

	return Result{Config: &out, Violations: violations}
}

// ApplyToSubConfig enforces locked settings on a sub-config.
func ApplyToSubConfig(sc *config.SubConfig, root *config.Config, p *config.OrgPolicy) *config.SubConfig {
	if p == nil || p.IsZero() || len(p.LockedSettings) == 0 {
		return sc
	}

	locked := make(map[string]bool, len(p.LockedSettings))
	for _, s := range p.LockedSettings {
		locked[strings.ToLower(strings.TrimSpace(s))] = true
	}

	out := *sc
	if locked["skip"] {
		out.Skip = nil
	}
	if locked["skip_func"] {
		out.SkipFunc = nil
	}
	if locked["exclude"] {
		out.Exclude = nil
	}
	if locked["include"] {
		out.Include = nil
	}
	if locked["tests"] {
		out.Tests = nil
	}
	if locked["operators"] {
		out.Operators = nil
	}
	if locked["threshold"] {
		out.Threshold = nil
	}
	return &out
}

func enforceRequiredOperators(
	current []string,
	required []string,
	allOps []mutator.Operator,
	violations []Violation,
) ([]string, []Violation) {
	// "all" means every operator is already active
	for _, c := range current {
		if strings.TrimSpace(c) == "all" {
			return current, violations
		}
	}

	existing := make(map[string]bool, len(current))
	for _, op := range current {
		existing[strings.TrimSpace(op)] = true
	}

	validOps := make(map[string]bool, len(allOps))
	for _, op := range allOps {
		validOps[op.Name()] = true
	}

	out := append([]string{}, current...)
	for _, req := range required {
		req = strings.TrimSpace(req)
		if req == "" {
			continue
		}
		if !validOps[req] {
			violations = append(violations, Violation{
				Setting:  "required_operators",
				Was:      "",
				Enforced: req,
				Reason:   "unknown operator name in org policy",
			})
			continue
		}
		if !existing[req] {
			violations = append(violations, Violation{
				Setting:  "operators",
				Was:      strings.Join(current, ","),
				Enforced: req + " (injected)",
				Reason:   "required by org policy",
			})
			out = append(out, req)
		}
	}
	return out, violations
}

func enforceForbiddenOperators(
	current []string,
	forbidden []string,
	violations []Violation,
) ([]string, []Violation) {
	deny := make(map[string]bool, len(forbidden))
	for _, f := range forbidden {
		deny[strings.TrimSpace(f)] = true
	}

	var out []string
	for _, op := range current {
		name := strings.TrimSpace(op)
		if deny[name] {
			violations = append(violations, Violation{
				Setting:  "operators",
				Was:      name,
				Enforced: "(removed)",
				Reason:   "forbidden by org policy",
			})
			continue
		}
		out = append(out, op)
	}
	return out, violations
}

func enforceMinConcurrent(current string, min int, violations []Violation) (string, []Violation) {
	effective := parseConcurrent(current)
	if effective >= min {
		return current, violations
	}
	violations = append(violations, Violation{
		Setting:  "concurrent",
		Was:      current,
		Enforced: strconv.Itoa(min),
		Reason:   fmt.Sprintf("below org min_concurrent (%d)", min),
	})
	return strconv.Itoa(min), violations
}

func parseConcurrent(val string) int {
	switch val {
	case "all":
		return runtime.NumCPU()
	case "half":
		n := runtime.NumCPU() / 2
		if n < 1 {
			return 1
		}
		return n
	default:
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 {
			return runtime.NumCPU()
		}
		return n
	}
}

func mergeUnique(base, additions []string) []string {
	seen := make(map[string]bool, len(base))
	for _, s := range base {
		seen[s] = true
	}
	out := append([]string{}, base...)
	for _, s := range additions {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
