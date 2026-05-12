//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aclfe/gorgon/internal/subconfig"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

// ============================================================================
// SUB-CONFIG RESOLUTION
//
// Tests cover the full chain-resolution contract: operator replace semantics
// (deepest wins), filter merge semantics (all levels accumulate), discovery
// rules (vendor excluded, root config excluded), and known bugs.
//
// All tests that need real mutation targets aim at internal/reporter so the
// production code exists and has identifiable mutant sites.
// ============================================================================

// TestSubConfig_NoSubConfigsDiscovered verifies that Discover on a directory
// tree with no gorgon.yml files returns an empty resolver. Entries() must be
// zero and HasAnyOverrides must be false, so callers can skip resolution
// overhead entirely.
func TestSubConfig_NoSubConfigsDiscovered(t *testing.T) {
	t.Skip("TODO: create a temp dir with Go source but no gorgon.yml; call " +
		"subconfig.Discover; assert Entries()==0 and HasAnyOverrides()==false")
}

// TestSubConfig_DiscoverFindsAllSubConfigs verifies that Discover walks the
// full tree and picks up every gorgon.yml in subdirectories, not just the
// nearest one. Create three nested sub-configs and assert Entries() == 3.
func TestSubConfig_DiscoverFindsAllSubConfigs(t *testing.T) {
	t.Skip("TODO: create a/gorgon.yml, a/b/gorgon.yml, a/b/c/gorgon.yml in a temp " +
		"tree; run Discover on the root; assert Entries()==3 and entries are sorted " +
		"shallowest-first by directory depth")
}

// TestSubConfig_RootConfigExcludedFromDiscovery verifies that when a
// gorgon.yml sits at the project root and is passed as rootConfigPath,
// Discover does not re-index it as a sub-config entry, preventing it from
// being applied twice in chain resolution.
func TestSubConfig_RootConfigExcludedFromDiscovery(t *testing.T) {
	t.Skip("TODO: create projectRoot/gorgon.yml and pass its path as rootConfigPath; " +
		"also create projectRoot/subdir/gorgon.yml; assert Entries()==1 (only subdir)")
}

// TestSubConfig_VendorDirectorySkipped verifies that gorgon.yml files inside
// vendor/ are never discovered. This prevents third-party code from injecting
// mutation settings into the host project.
func TestSubConfig_VendorDirectorySkipped(t *testing.T) {
	t.Skip("TODO: create projectRoot/vendor/pkg/gorgon.yml and an unrelated " +
		"projectRoot/subdir/gorgon.yml; run Discover; assert vendor entry is absent " +
		"from Entries() while subdir entry is present")
}

// TestSubConfig_OperatorReplaceSemantics_DeepestWins verifies that when
// multiple sub-configs in the ancestor chain each specify operators, the
// deepest (most-specific) one wins and the shallower ones are ignored.
// e.g. a/gorgon.yml sets [negate_condition], a/b/gorgon.yml sets
// [arithmetic_flip]; a file in a/b/ should use arithmetic_flip only.
func TestSubConfig_OperatorReplaceSemantics_DeepestWins(t *testing.T) {
	t.Skip("TODO: build a two-level resolver with different operator lists at each level; " +
		"call EffectiveOperators for a file in the deeper dir; assert only the " +
		"deeper operator list is returned, not the shallower one")
}

// TestSubConfig_OperatorReplace_FallsBackToBase_WhenChainEmpty verifies that
// when no sub-config in the chain specifies operators, EffectiveOperators
// returns the base operator list unchanged.
func TestSubConfig_OperatorReplace_FallsBackToBase_WhenChainEmpty(t *testing.T) {
	t.Skip("TODO: discover a tree with sub-configs that have no operators field; " +
		"call EffectiveOperators(file, baseOps, allOps) and assert baseOps returned verbatim")
}

// TestSubConfig_OperatorAllShorthand_InSubConfig verifies that using
// operators: [all] in a sub-config resolves to the full registered operator
// set, not the literal string "all".
func TestSubConfig_OperatorAllShorthand_InSubConfig(t *testing.T) {
	allOps := mutator.ListAll()
	if len(allOps) == 0 {
		t.Fatal("no operators registered — blank imports missing from test binary")
	}
	_ = allOps
	t.Skip("TODO: construct a Resolver with a sub-config that has Operators:[\"all\"]; " +
		"call EffectiveOperators for a file inside that dir; assert len(result)==len(allOps)")
}

// TestSubConfig_ThresholdReplaceSemantics_DeepestWins verifies replace
// semantics for Threshold: when both a shallow and a deep sub-config set a
// threshold, the deepest wins for files in the deep directory.
func TestSubConfig_ThresholdReplaceSemantics_DeepestWins(t *testing.T) {
	shallow := 50.0
	deep := 90.0
	_ = shallow
	_ = deep
	t.Skip("TODO: build resolver with Threshold=50.0 at shallow level and Threshold=90.0 at " +
		"deep level; call EffectiveThreshold for a file in deep dir; assert result==90.0")
}

// TestSubConfig_ThresholdFallsBackToRoot_WhenChainEmpty verifies that
// EffectiveThreshold returns rootThreshold when no sub-config in the chain
// has a threshold pointer set (nil means unspecified).
func TestSubConfig_ThresholdFallsBackToRoot_WhenChainEmpty(t *testing.T) {
	t.Skip("TODO: discover a tree where sub-configs all have Threshold==nil; " +
		"call EffectiveThreshold(file, 75.0) and assert 75.0 is returned")
}

// TestSubConfig_SkipMergesAcrossChain verifies merge semantics: skip lists
// from every level in the ancestor chain are combined. A file in a/b/ should
// see skip entries from the root config, a/gorgon.yml, and a/b/gorgon.yml
// all merged together with no entries dropped.
func TestSubConfig_SkipMergesAcrossChain(t *testing.T) {
	t.Skip("TODO: build a 3-level chain each with a unique skip entry; " +
		"call EffectiveFilters(file, rootCfg); assert all three skip entries appear " +
		"in the returned skip slice (root contributes first, then shallow, then deep)")
}

// TestSubConfig_ExcludeMergesAcrossChain verifies that exclude entries
// accumulate across the chain (merge semantics).
func TestSubConfig_ExcludeMergesAcrossChain(t *testing.T) {
	t.Skip("TODO: 3-level chain each with a different exclude pattern; " +
		"call EffectiveFilters; assert all three exclude patterns present in result")
}

// TestSubConfig_IncludeMergesAcrossChain verifies that include entries
// accumulate across the chain (merge semantics).
func TestSubConfig_IncludeMergesAcrossChain(t *testing.T) {
	t.Skip("TODO: 3-level chain each with a different include entry; " +
		"call EffectiveFilters; assert all include entries are merged")
}

// TestSubConfig_SkipFuncMergesAcrossChain verifies that skip_func entries
// accumulate across the chain (merge semantics), preserving file:func format.
func TestSubConfig_SkipFuncMergesAcrossChain(t *testing.T) {
	t.Skip("TODO: 3-level chain each with a unique skip_func entry; " +
		"call EffectiveFilters; assert all skip_func entries are present in result")
}

// TestSubConfig_DirRulesMergeAcrossChain verifies that dir_rules from every
// level in the chain are merged (not replaced). Multiple levels contributing
// rules for different directories should each appear in the effective list.
func TestSubConfig_DirRulesMergeAcrossChain(t *testing.T) {
	t.Skip("TODO: 2-level chain — shallow level defines dir_rules for dirA, deep level " +
		"for dirB; call EffectiveDirRules; assert both rule entries appear in the result")
}

// TestSubConfig_SuppressNeverApplied exposes the known bug where
// EffectiveSuppress is defined in resolver.go but is never called anywhere
// in production code. runner.Run calls eng.SetSuppressEntries(cfg.Suppress)
// from the root config only. Suppress entries placed in sub-configs are
// silently ignored.
//
// Expected behavior: mutant at a suppressed location should be absent.
// Actual behavior: suppress entry in sub-config has no effect.
//
// BUG: resolver.EffectiveSuppress has zero callers in production code.
func TestSubConfig_SuppressNeverApplied(t *testing.T) {
	t.Skip("TODO: write internal/baseline/gorgon.yml with a suppress entry pointing " +
		"at a known mutant location; run generateMutantsWithConfig targeting " +
		"internal/baseline; assert the suppressed mutant is absent. " +
		"Currently FAILS because EffectiveSuppress is defined but never called — " +
		"sub-config suppress entries are silently ignored. " +
		"Clean up the gorgon.yml with t.Cleanup")
}

// TestSubConfig_EffectiveTests_DeepestWins verifies replace semantics for the
// Tests field: when a sub-config specifies a tests list, that list replaces
// the root tests for files in that directory.
func TestSubConfig_EffectiveTests_DeepestWins(t *testing.T) {
	t.Skip("TODO: build resolver where a sub-config specifies a non-empty tests list; " +
		"call EffectiveTests(file, rootTests) and assert the sub-config list is returned, " +
		"not rootTests")
}

// TestSubConfig_EffectiveTests_FallsBackToRoot_WhenChainEmpty verifies that
// EffectiveTests returns rootTests when no sub-config in the chain specifies
// a tests list.
func TestSubConfig_EffectiveTests_FallsBackToRoot_WhenChainEmpty(t *testing.T) {
	t.Skip("TODO: discover a tree with sub-configs that have no tests field; " +
		"call EffectiveTests(file, rootTests) and assert rootTests is returned verbatim")
}

// TestSubConfig_PolicyLocked_OperatorsNulled verifies that when an org policy
// marks 'operators' as a locked setting, DiscoverWithPolicy threads that
// through the resolver so that sub-config operator fields are nulled out and
// EffectiveOperators returns the base list (not the sub-config override).
func TestSubConfig_PolicyLocked_OperatorsNulled(t *testing.T) {
	_ = subconfig.DiscoverWithPolicy
	t.Skip("TODO: create a sub-config with a specific operator list; " +
		"call DiscoverWithPolicy with policy.LockedSettings=[\"operators\"]; " +
		"call EffectiveOperators for a file in that dir; " +
		"assert sub-config operators are nulled and the base list is returned instead")
}

// TestSubConfig_PolicyLocked_SkipNulled verifies that locking 'skip' in the
// org policy prevents sub-config skip entries from accumulating in
// EffectiveFilters.
func TestSubConfig_PolicyLocked_SkipNulled(t *testing.T) {
	t.Skip("TODO: create a sub-config with skip entries; use DiscoverWithPolicy with " +
		"LockedSettings=[\"skip\"]; call EffectiveFilters; assert sub-config skip " +
		"entries are absent from the result (nulled by ApplyToSubConfig inside chain())")
}

// TestSubConfig_FilterSites_RespectsSubConfigSkip verifies end-to-end that
// when a sub-config inside a target directory specifies skip: [reporter.go],
// runner.FilterSites drops all mutants from that file. This is the integration
// boundary between the resolver and the FilterSites call in runner.
func TestSubConfig_FilterSites_RespectsSubConfigSkip(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "internal/reporter")

	rawByFile := mutantsByFile(generateMutantsRaw(t, targetDir))
	if rawByFile["reporter.go"] == 0 {
		t.Fatal("internal/reporter/reporter.go has no mutants — cannot test skip behavior")
	}

	subCfgPath := filepath.Join(repoRoot, "internal/reporter/gorgon.yml")
	if _, err := os.Stat(subCfgPath); err == nil {
		t.Skip("internal/reporter/gorgon.yml already exists — would interfere with test; " +
			"remove it or adapt the test to use a different target")
	}

	t.Skip("TODO: write internal/reporter/gorgon.yml with skip: [reporter.go]; " +
		"call generateMutantsWithConfig targeting internal/reporter; " +
		"assert filtered['reporter.go']==0 while other files still have mutants; " +
		"remove the gorgon.yml with t.Cleanup")
}

// TestSubConfig_SubConfigLoadParseError verifies that Discover surfaces a
// parse error when a gorgon.yml exists but contains invalid YAML, rather than
// silently skipping it. The caller must be able to detect malformed configs.
func TestSubConfig_SubConfigLoadParseError(t *testing.T) {
	_ = config.LoadSubConfig
	t.Skip("TODO: create a temp gorgon.yml with invalid YAML content; " +
		"call subconfig.Discover; assert an error is returned (not nil)")
}
