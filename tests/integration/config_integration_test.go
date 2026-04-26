//go:build integration
// +build integration

package integration

import "testing"

// ============================================================================
// SUPPRESSIONS
// ============================================================================

// TestSuppressions_InlineComment verifies //gorgon:ignore suppresses next line
func TestSuppressions_InlineComment(t *testing.T) {
	t.Skip("TODO: Verify //gorgon:ignore suppresses mutations on next line")
}

// TestSuppressions_InlineComment_SpecificOperator verifies //gorgon:ignore operator_name
func TestSuppressions_InlineComment_SpecificOperator(t *testing.T) {
	t.Skip("TODO: Verify //gorgon:ignore arithmetic_flip only suppresses that operator")
}

// TestSuppressions_InlineComment_MultipleOperators verifies //gorgon:ignore op1,op2
func TestSuppressions_InlineComment_MultipleOperators(t *testing.T) {
	t.Skip("TODO: Verify //gorgon:ignore arithmetic_flip,panic_removal suppresses both")
}

// TestSuppressions_InlineComment_WithLineNumber verifies //gorgon:ignore operator:line
func TestSuppressions_InlineComment_WithLineNumber(t *testing.T) {
	t.Skip("TODO: Verify //gorgon:ignore arithmetic_flip:9 suppresses specific line")
}

// TestSuppressions_ConfigFile verifies suppress: in config file works
func TestSuppressions_ConfigFile(t *testing.T) {
	t.Skip("TODO: Verify suppress: [{location: file.go:5, operators: [arithmetic_flip]}]")
}

// TestSuppressions_ConfigFile_AllOperators verifies suppressing all operators on a line
func TestSuppressions_ConfigFile_AllOperators(t *testing.T) {
	t.Skip("TODO: Verify suppress: [{location: file.go:10}] suppresses all operators")
}

// TestSuppressions_AutoSync verifies inline comments are synced to config file
func TestSuppressions_AutoSync(t *testing.T) {
	t.Skip("TODO: Verify //gorgon:ignore is added to config suppress: section")
}

// TestSuppressions_AutoSync_PreservesExisting verifies auto-sync preserves existing suppressions
func TestSuppressions_AutoSync_PreservesExisting(t *testing.T) {
	t.Skip("TODO: Verify existing config suppressions are preserved during auto-sync")
}

// TestSuppressions_AutoSync_RelativePaths verifies auto-sync uses relative paths
func TestSuppressions_AutoSync_RelativePaths(t *testing.T) {
	t.Skip("TODO: Verify auto-synced paths are relative to project root")
}

// TestSuppressions_AutoSync_OnlyWithConfig verifies auto-sync only happens with -config
func TestSuppressions_AutoSync_OnlyWithConfig(t *testing.T) {
	t.Skip("TODO: Verify auto-sync doesn't happen without -config flag")
}

// TestSuppressions_BaseOverride verifies base: in config changes path resolution
func TestSuppressions_BaseOverride(t *testing.T) {
	t.Skip("TODO: Verify base: examples makes paths relative to examples/ instead of go.mod")
}

// TestSuppressions_MergeInlineAndConfig verifies inline and config suppressions merge
func TestSuppressions_MergeInlineAndConfig(t *testing.T) {
	t.Skip("TODO: Verify both inline and config suppressions are applied")
}

// ============================================================================
// SUB-CONFIGS
// ============================================================================

// TestSubConfig_Discovery verifies sub-configs are discovered in subdirectories
func TestSubConfig_Discovery(t *testing.T) {
	t.Skip("TODO: Verify gorgon.yml files in subdirectories are discovered")
}

// TestSubConfig_Discovery_SkipsVendor verifies vendor/ is skipped during discovery
func TestSubConfig_Discovery_SkipsVendor(t *testing.T) {
	t.Skip("TODO: Verify vendor/ directories are skipped")
}

// TestSubConfig_Discovery_SkipsGit verifies .git/ is skipped during discovery
func TestSubConfig_Discovery_SkipsGit(t *testing.T) {
	t.Skip("TODO: Verify .git/ directories are skipped")
}

// TestSubConfig_Discovery_SkipsUnderscore verifies _prefixed dirs are skipped
func TestSubConfig_Discovery_SkipsUnderscore(t *testing.T) {
	t.Skip("TODO: Verify _internal/ directories are skipped")
}

// TestSubConfig_Chaining verifies sub-config chain resolution
func TestSubConfig_Chaining(t *testing.T) {
	t.Skip("TODO: Verify root → core/gorgon.yml → core/auth/gorgon.yml chain")
}

// TestSubConfig_Chaining_ThreeLevels verifies three-level chain resolution
func TestSubConfig_Chaining_ThreeLevels(t *testing.T) {
	t.Skip("TODO: Verify root → level1 → level2 → level3 chain resolution")
}

// TestSubConfig_Operators_Replace verifies operators field uses replace semantics
func TestSubConfig_Operators_Replace(t *testing.T) {
	t.Skip("TODO: Verify deepest sub-config operators completely replace parent")
}

// TestSubConfig_Threshold_Replace verifies threshold field uses replace semantics
func TestSubConfig_Threshold_Replace(t *testing.T) {
	t.Skip("TODO: Verify deepest sub-config threshold replaces parent")
}

// TestSubConfig_Tests_Replace verifies tests field uses replace semantics
func TestSubConfig_Tests_Replace(t *testing.T) {
	t.Skip("TODO: Verify deepest sub-config tests replace parent")
}

// TestSubConfig_Concurrent_Replace verifies concurrent field uses replace semantics
func TestSubConfig_Concurrent_Replace(t *testing.T) {
	t.Skip("TODO: Verify deepest sub-config concurrent replaces parent")
}

// TestSubConfig_Exclude_Merge verifies exclude field uses merge semantics
func TestSubConfig_Exclude_Merge(t *testing.T) {
	t.Skip("TODO: Verify exclude patterns accumulate across chain")
}

// TestSubConfig_Include_Merge verifies include field uses merge semantics
func TestSubConfig_Include_Merge(t *testing.T) {
	t.Skip("TODO: Verify include patterns accumulate across chain")
}

// TestSubConfig_Skip_Merge verifies skip field uses merge semantics
func TestSubConfig_Skip_Merge(t *testing.T) {
	t.Skip("TODO: Verify skip paths accumulate across chain")
}

// TestSubConfig_SkipFunc_Merge verifies skip_func field uses merge semantics
func TestSubConfig_SkipFunc_Merge(t *testing.T) {
	t.Skip("TODO: Verify skip_func entries accumulate across chain")
}

// TestSubConfig_Suppress_Merge verifies suppress field uses merge semantics
func TestSubConfig_Suppress_Merge(t *testing.T) {
	t.Skip("TODO: Verify suppressions accumulate across chain")
}

// TestSubConfig_DirRules_Merge verifies dir_rules field uses merge semantics
func TestSubConfig_DirRules_Merge(t *testing.T) {
	t.Skip("TODO: Verify dir_rules accumulate across chain")
}

// TestSubConfig_PerPackageThreshold verifies per-package threshold checking
func TestSubConfig_PerPackageThreshold(t *testing.T) {
	t.Skip("TODO: Verify each package is checked against its own threshold")
}

// TestSubConfig_PerPackageThreshold_FailureReport verifies failed packages are reported
func TestSubConfig_PerPackageThreshold_FailureReport(t *testing.T) {
	t.Skip("TODO: Verify 'Packages below threshold:' section lists failed packages")
}

// TestSubConfig_PerPackageThreshold_MultipleFailures verifies multiple package failures
func TestSubConfig_PerPackageThreshold_MultipleFailures(t *testing.T) {
	t.Skip("TODO: Verify multiple packages can fail threshold check")
}

// TestSubConfig_Mode_Merge verifies sub_config_mode: merge behavior
func TestSubConfig_Mode_Merge(t *testing.T) {
	t.Skip("TODO: Verify merge mode accumulates settings from parent")
}

// TestSubConfig_Mode_Replace verifies sub_config_mode: replace behavior
func TestSubConfig_Mode_Replace(t *testing.T) {
	t.Skip("TODO: Verify replace mode ignores parent settings")
}

// TestSubConfig_Mode_Isolate verifies sub_config_mode: isolate behavior
func TestSubConfig_Mode_Isolate(t *testing.T) {
	t.Skip("TODO: Verify isolate mode completely isolates sub-config")
}

// TestSubConfig_ThresholdInherit verifies threshold_inherit propagation
func TestSubConfig_ThresholdInherit(t *testing.T) {
	t.Skip("TODO: Verify threshold_inherit: true propagates root threshold to sub-configs")
}

// TestSubConfig_ThresholdInherit_False verifies threshold_inherit: false behavior
func TestSubConfig_ThresholdInherit_False(t *testing.T) {
	t.Skip("TODO: Verify threshold_inherit: false doesn't propagate threshold")
}

// TestSubConfig_ThresholdInherit_WithSubConfigThreshold verifies explicit threshold wins
func TestSubConfig_ThresholdInherit_WithSubConfigThreshold(t *testing.T) {
	t.Skip("TODO: Verify explicit sub-config threshold overrides inherited threshold")
}

// ============================================================================
// DIR RULES
// ============================================================================

// TestDirRules_Whitelist verifies whitelist restricts operators
func TestDirRules_Whitelist(t *testing.T) {
	t.Skip("TODO: Verify whitelist: [arithmetic_flip] only allows that operator")
}

// TestDirRules_Blacklist verifies blacklist excludes operators
func TestDirRules_Blacklist(t *testing.T) {
	t.Skip("TODO: Verify blacklist: [defer_removal] excludes that operator")
}

// TestDirRules_BlacklistAll verifies blacklist: [all] skips entire directory
func TestDirRules_BlacklistAll(t *testing.T) {
	t.Skip("TODO: Verify blacklist: [all] skips entire directory")
}

// TestDirRules_WhitelistPrecedence verifies whitelist takes precedence over blacklist
func TestDirRules_WhitelistPrecedence(t *testing.T) {
	t.Skip("TODO: Verify whitelist overrides blacklist when both present")
}

// TestDirRules_LongestPrefixMatch verifies longest-prefix matching
func TestDirRules_LongestPrefixMatch(t *testing.T) {
	t.Skip("TODO: Verify internal/core/auth uses internal/core/auth rule, not internal/core")
}

// TestDirRules_LongestPrefixMatch_ThreeLevels verifies three-level prefix matching
func TestDirRules_LongestPrefixMatch_ThreeLevels(t *testing.T) {
	t.Skip("TODO: Verify a/b/c uses most specific rule among a, a/b, a/b/c")
}

// TestDirRules_NoMatch verifies default behavior when no rule matches
func TestDirRules_NoMatch(t *testing.T) {
	t.Skip("TODO: Verify all operators apply when no dir_rule matches")
}

// TestDirRules_MergeAcrossSubConfigs verifies dir_rules merge across sub-configs
func TestDirRules_MergeAcrossSubConfigs(t *testing.T) {
	t.Skip("TODO: Verify dir_rules from root and sub-configs are merged")
}

// ============================================================================
// ORG POLICY
// ============================================================================

// TestOrgPolicy_Discovery_EnvVar verifies GORGON_ORG_POLICY env var discovery
func TestOrgPolicy_Discovery_EnvVar(t *testing.T) {
	t.Skip("TODO: Verify GORGON_ORG_POLICY env var points to org policy file")
}

// TestOrgPolicy_Discovery_WalkUp verifies walking up directory tree
func TestOrgPolicy_Discovery_WalkUp(t *testing.T) {
	t.Skip("TODO: Verify gorgon-org.yml is found by walking up from project root")
}

// TestOrgPolicy_Discovery_XDGConfig verifies XDG_CONFIG_HOME discovery
func TestOrgPolicy_Discovery_XDGConfig(t *testing.T) {
	t.Skip("TODO: Verify $XDG_CONFIG_HOME/gorgon/gorgon-org.yml is checked")
}

// TestOrgPolicy_Discovery_Priority verifies discovery priority order
func TestOrgPolicy_Discovery_Priority(t *testing.T) {
	t.Skip("TODO: Verify env var > walk up > XDG priority order")
}

// TestOrgPolicy_ThresholdFloor verifies threshold_floor enforcement
func TestOrgPolicy_ThresholdFloor(t *testing.T) {
	t.Skip("TODO: Verify threshold below threshold_floor is raised")
}

// TestOrgPolicy_ThresholdFloor_SubConfigs verifies threshold_floor applies to sub-configs
func TestOrgPolicy_ThresholdFloor_SubConfigs(t *testing.T) {
	t.Skip("TODO: Verify sub-config thresholds are also raised to floor")
}

// TestOrgPolicy_RequiredOperators verifies required_operators injection
func TestOrgPolicy_RequiredOperators(t *testing.T) {
	t.Skip("TODO: Verify required_operators are injected into all configs")
}

// TestOrgPolicy_RequiredOperators_MergeWithExisting verifies required ops merge with existing
func TestOrgPolicy_RequiredOperators_MergeWithExisting(t *testing.T) {
	t.Skip("TODO: Verify required_operators are added to existing operators")
}

// TestOrgPolicy_ForbiddenOperators verifies forbidden_operators removal
func TestOrgPolicy_ForbiddenOperators(t *testing.T) {
	t.Skip("TODO: Verify forbidden_operators are removed from all configs")
}

// TestOrgPolicy_ForbiddenOperators_OverridesRequired verifies forbidden overrides required
func TestOrgPolicy_ForbiddenOperators_OverridesRequired(t *testing.T) {
	t.Skip("TODO: Verify forbidden_operators removes even required_operators")
}

// TestOrgPolicy_LockedSettings_Skip verifies skip cannot be overridden
func TestOrgPolicy_LockedSettings_Skip(t *testing.T) {
	t.Skip("TODO: Verify locked_settings: [skip] prevents teams from using skip")
}

// TestOrgPolicy_LockedSettings_SkipFunc verifies skip_func cannot be overridden
func TestOrgPolicy_LockedSettings_SkipFunc(t *testing.T) {
	t.Skip("TODO: Verify locked_settings: [skip_func] prevents function exemptions")
}

// TestOrgPolicy_LockedSettings_Exclude verifies exclude cannot be overridden
func TestOrgPolicy_LockedSettings_Exclude(t *testing.T) {
	t.Skip("TODO: Verify locked_settings: [exclude] prevents file exclusions")
}

// TestOrgPolicy_LockedSettings_Cache verifies cache cannot be disabled
func TestOrgPolicy_LockedSettings_Cache(t *testing.T) {
	t.Skip("TODO: Verify locked_settings: [cache] prevents disabling cache")
}

// TestOrgPolicy_LockedSettings_Operators verifies operators cannot be changed
func TestOrgPolicy_LockedSettings_Operators(t *testing.T) {
	t.Skip("TODO: Verify locked_settings: [operators] prevents operator changes")
}

// TestOrgPolicy_LockedSettings_Threshold verifies threshold cannot be lowered
func TestOrgPolicy_LockedSettings_Threshold(t *testing.T) {
	t.Skip("TODO: Verify locked_settings: [threshold] prevents threshold changes")
}

// TestOrgPolicy_LockedSettings_ViolationMode verifies violation_mode cannot be changed
func TestOrgPolicy_LockedSettings_ViolationMode(t *testing.T) {
	t.Skip("TODO: Verify locked_settings: [violation_mode] prevents silencing violations")
}

// TestOrgPolicy_ForcedSkipPaths verifies forced_skip_paths are always skipped
func TestOrgPolicy_ForcedSkipPaths(t *testing.T) {
	t.Skip("TODO: Verify forced_skip_paths: ['*.pb.go'] skips generated code")
}

// TestOrgPolicy_ForcedExcludePatterns verifies forced_exclude_patterns are always excluded
func TestOrgPolicy_ForcedExcludePatterns(t *testing.T) {
	t.Skip("TODO: Verify forced_exclude_patterns are applied everywhere")
}

// TestOrgPolicy_MinConcurrent verifies min_concurrent enforcement
func TestOrgPolicy_MinConcurrent(t *testing.T) {
	t.Skip("TODO: Verify concurrent below min_concurrent is raised")
}

// TestOrgPolicy_RequireCache verifies require_cache enforcement
func TestOrgPolicy_RequireCache(t *testing.T) {
	t.Skip("TODO: Verify require_cache: true forces cache to be enabled")
}

// TestOrgPolicy_ViolationMode_Fail verifies violation_mode: fail behavior
func TestOrgPolicy_ViolationMode_Fail(t *testing.T) {
	t.Skip("TODO: Verify violation_mode: fail exits with error on violations")
}

// TestOrgPolicy_ViolationMode_Warn verifies violation_mode: warn behavior
func TestOrgPolicy_ViolationMode_Warn(t *testing.T) {
	t.Skip("TODO: Verify violation_mode: warn logs violations but continues")
}

// TestOrgPolicy_ViolationMode_Silent verifies violation_mode: silent behavior
func TestOrgPolicy_ViolationMode_Silent(t *testing.T) {
	t.Skip("TODO: Verify violation_mode: silent applies policy without logging")
}

// TestOrgPolicy_ViolationReporting verifies violation messages are logged
func TestOrgPolicy_ViolationReporting(t *testing.T) {
	t.Skip("TODO: Verify 'Org policy applied N constraint(s):' message is shown")
}

// TestOrgPolicy_ViolationReporting_Details verifies violation details are logged
func TestOrgPolicy_ViolationReporting_Details(t *testing.T) {
	t.Skip("TODO: Verify each violation shows what was changed and why")
}

// TestOrgPolicy_WithSubConfigs verifies org policy applies to sub-configs
func TestOrgPolicy_WithSubConfigs(t *testing.T) {
	t.Skip("TODO: Verify org policy constraints apply to all sub-configs")
}

// TestOrgPolicy_WithWorkspaces verifies org policy applies to workspace modules
func TestOrgPolicy_WithWorkspaces(t *testing.T) {
	t.Skip("TODO: Verify org policy applies to all modules in workspace")
}
