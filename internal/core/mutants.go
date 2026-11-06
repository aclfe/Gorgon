package testing

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/internal/subconfig"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

// Mutant status constants — single source of truth.
const (
	StatusKilled       = "killed"
	StatusSurvived     = "survived"
	StatusUntested     = "untested"
	StatusInvalid      = "invalid"
	StatusTimeout      = "timeout"
	StatusError        = "error"
	StatusCompileError = "compile_error"
)

func IsTerminalPreRun(status string) bool {
	return status == StatusInvalid || status == StatusCompileError
}

type Mutant struct {
	ID           int
	Site         engine.Site
	Operator     mutator.Operator
	TempDir      string
	TempLine     int
	TempCol      int
	Status       string
	Error        error
	KilledBy     string
	KillDuration time.Duration
	KillOutput   string
	ErrorReason  string
}

func effectiveOperators(filePath, projectRoot string, base []mutator.Operator, rules []config.DirOperatorRule, log *logger.Logger) []mutator.Operator {
	if len(rules) == 0 {
		return base
	}

	rel, err := filepath.Rel(projectRoot, filePath)
	if err != nil {
		return base
	}

	// Find longest-prefix (most specific) matching rule
	bestLen := -1
	var best *config.DirOperatorRule
	for i := range rules {
		r := &rules[i]
		dir := filepath.Clean(r.Dir)
		if rel == dir || strings.HasPrefix(rel, dir+string(filepath.Separator)) {
			if len(dir) > bestLen {
				bestLen = len(dir)
				best = r
			}
		}
	}

	if best == nil {
		return base
	}

	if best.Whitelist != nil {
		allow := make(map[string]bool, len(best.Whitelist))
		for _, op := range best.Whitelist {
			allow[strings.TrimSpace(op)] = true
		}
		var out []mutator.Operator
		for _, op := range base {
			if allow[op.Name()] {
				out = append(out, op)
			}
		}
		return out
	}

	if len(best.Blacklist) > 0 {
		// Check for "all" shorthand
		for _, op := range best.Blacklist {
			if strings.TrimSpace(op) == "all" {
				if log != nil && log.IsDebug() {
					log.Debug("Dir rule %s: blacklist all operators for %s", best.Dir, rel)
				}
				return nil
			}
		}
		deny := make(map[string]bool, len(best.Blacklist))
		for _, op := range best.Blacklist {
			deny[strings.TrimSpace(op)] = true
		}
		var out []mutator.Operator
		for _, op := range base {
			if !deny[op.Name()] {
				out = append(out, op)
			}
		}
		if log != nil && log.IsDebug() {
			log.Debug("Dir rule %s: blacklist %d operators for %s", best.Dir, len(base)-len(out), rel)
		}
		return out
	}

	return base
}

func GenerateMutants(sites []engine.Site, operators []mutator.Operator, allOps []mutator.Operator, projectRoot string, dirRules []config.DirOperatorRule, resolver *subconfig.Resolver, log *logger.Logger) []Mutant {
	sorted := make([]engine.Site, len(sites))
	copy(sorted, sites)
	engine.SortSites(sorted)

	seen := make(map[engine.SiteKey]bool, len(sorted))
	mutants := make([]Mutant, 0, len(sorted))
	mutantID := 1

	subSuppress := newSubconfigSuppressions(resolver, projectRoot)
	seenEffects := make(map[string]bool)

	for _, site := range sorted {
		if site.Node == nil || site.File == nil {
			continue
		}
		key := site.Key()
		if seen[key] {
			continue
		}
		seen[key] = true

		// 1. Apply sub-config operator override (replace semantics)
		ops := operators
		if resolver != nil && resolver.HasAnyOverrides() {
			ops = resolver.EffectiveOperators(site.File.Name(), operators, allOps)
		}

		// 2. Apply dir_rules on top (existing logic, but with merged rules)
		effectiveDirRules := dirRules
		if resolver != nil && resolver.HasAnyOverrides() {
			effectiveDirRules = resolver.EffectiveDirRules(site.File.Name(), dirRules)
		}
		ops = effectiveOperators(site.File.Name(), projectRoot, ops, effectiveDirRules, log)
		if len(ops) == 0 {
			continue
		}

		ops = sortedUniqueOperators(ops)
		var chosen []mutator.Operator
		for _, op := range ops {
			if !siteAllows(site, op) {
				continue
			}
			if subSuppress.ignored(site, op.Name()) {
				continue
			}
			mutated, changed := mutation(op, site)
			if !changed {
				continue
			}
			if key, ok := effectKey(site, mutated); ok {
				if seenEffects[key] {
					continue
				}
				seenEffects[key] = true
			}
			chosen = append(chosen, op)
		}
		chosen = dropEquivalentConditionOps(chosen)

		for _, op := range chosen {
			mutants = append(mutants, Mutant{
				ID:       mutantID,
				Site:     site,
				Operator: op,
			})
			mutantID++
		}
	}
	return mutants
}

func siteAllows(site engine.Site, op mutator.Operator) bool {
	name := op.Name()
	if site.Ops != nil && site.EvaluatedOps[name] {
		for _, n := range site.Ops {
			if n == name {
				return true
			}
		}
		return false
	}
	if !canApply(op, site) {
		return false
	}
	return !engine.IsIgnored(site.Directives, site.Line, name, site.Column)
}

func sortedUniqueOperators(ops []mutator.Operator) []mutator.Operator {
	out := make([]mutator.Operator, 0, len(ops))
	seen := make(map[string]bool, len(ops))
	for _, op := range ops {
		if op == nil || seen[op.Name()] {
			continue
		}
		seen[op.Name()] = true
		out = append(out, op)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

func SiteContext(site engine.Site, op mutator.Operator) mutator.Context {
	ctx := mutator.Context{
		ReturnType:    site.ReturnType,
		FunctionName:  site.FunctionName,
		File:          site.FileAST,
		EnclosingFunc: site.EnclosingFunc,
		Parent:        site.Parent,
	}
	if op != nil && site.Hints != nil {
		ctx.Hint = site.Hints[op.Name()]
	}
	return ctx
}

func mutation(op mutator.Operator, site engine.Site) (ast.Node, bool) {
	mutated := mutator.ApplyOperatorWithContext(op, site.Node, SiteContext(site, op))
	if mutated == nil {
		return nil, false
	}
	return mutated, printNode(mutated) != printNode(site.Node)
}

func effectKey(site engine.Site, mutated ast.Node) (string, bool) {
	if site.File == nil {
		return "", false
	}
	if _, ok := site.Node.(*ast.ReturnStmt); ok {
		return fmt.Sprintf("%s:%d:%d:%s", site.File.Name(), site.Offset, site.EndOffset, printNode(mutated)), true
	}
	ret, ok := site.Parent.(*ast.ReturnStmt)
	if !ok {
		return "", false
	}
	expr, ok := mutated.(ast.Expr)
	if !ok || !ret.Pos().IsValid() {
		return "", false
	}
	cp := *ret
	cp.Results = make([]ast.Expr, len(ret.Results))
	replaced := false
	for i, r := range ret.Results {
		if r == site.Node {
			cp.Results[i] = expr
			replaced = true
		} else {
			cp.Results[i] = r
		}
	}
	if !replaced {
		return "", false
	}
	start, end := site.File.Offset(ret.Pos()), site.File.Offset(ret.End())
	return fmt.Sprintf("%s:%d:%d:%s", site.File.Name(), start, end, printNode(&cp)), true
}

func printNode(n ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), n); err != nil {
		return fmt.Sprintf("%p", n)
	}
	return buf.String()
}

func dropEquivalentConditionOps(ops []mutator.Operator) []mutator.Operator {
	generic := func(name string) bool {
		return strings.HasPrefix(name, "if_condition_") || strings.HasPrefix(name, "for_condition_")
	}
	ordered := make([]mutator.Operator, 0, len(ops))
	for _, op := range ops {
		if !generic(op.Name()) {
			ordered = append(ordered, op)
		}
	}
	for _, op := range ops {
		if generic(op.Name()) {
			ordered = append(ordered, op)
		}
	}
	seenKind := make(map[mutator.ConditionKind]bool)
	keep := make(map[string]bool, len(ops))
	for _, op := range ordered {
		if cop, ok := op.(mutator.ConditionOperator); ok {
			kind := cop.ConditionKind()
			if kind != mutator.ConditionNone {
				if seenKind[kind] {
					continue
				}
				seenKind[kind] = true
			}
		}
		keep[op.Name()] = true
	}
	out := make([]mutator.Operator, 0, len(ops))
	for _, op := range ops {
		if keep[op.Name()] {
			out = append(out, op)
		}
	}
	return out
}

type subconfigSuppressions struct {
	resolver *subconfig.Resolver
	root     string
	byFile   map[string]map[int]map[string]map[int]bool
}

func newSubconfigSuppressions(resolver *subconfig.Resolver, root string) *subconfigSuppressions {
	if resolver == nil || !resolver.HasAnyOverrides() {
		return nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	return &subconfigSuppressions{resolver: resolver, root: abs, byFile: make(map[string]map[int]map[string]map[int]bool)}
}

func (s *subconfigSuppressions) ignored(site engine.Site, op string) bool {
	if s == nil {
		return false
	}
	file := site.File.Name()
	abs, err := filepath.Abs(file)
	if err != nil {
		abs = file
	}
	d, ok := s.byFile[abs]
	if !ok {
		entries := s.resolver.EffectiveSuppress(file, nil)
		d = engine.BuildDirectives(entries, s.root)[abs]
		s.byFile[abs] = d
	}
	return engine.IsIgnored(d, site.Line, op, site.Column)
}

func canApply(op mutator.Operator, site engine.Site) bool {
	return mutator.CanApplyWithContext(op, site.Node, SiteContext(site, op))
}
