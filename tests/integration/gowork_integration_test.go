//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aclfe/gorgon/internal/gowork"
)

// TestGoWork_Find_ReturnsNil_WhenNoGoWork verifies that Find returns nil
// when there is no go.work file anywhere in the directory tree above startDir.
func TestGoWork_Find_ReturnsNil_WhenNoGoWork(t *testing.T) {
	dir := t.TempDir()
	if ws := gowork.Find(dir); ws != nil {
		t.Errorf("expected nil, got workspace with root %s and modules %v", ws.Root, ws.Modules)
	}
}

// TestGoWork_Find_ReturnsWorkspace_WhenGoWorkInStartDir verifies that Find
// returns a non-nil Workspace when go.work sits directly in startDir.
func TestGoWork_Find_ReturnsWorkspace_WhenGoWorkInStartDir(t *testing.T) {
	dir := t.TempDir()
	content := "go 1.21\n\nuse (\n\t./mod\n)\n"
	if err := os.WriteFile(filepath.Join(dir, "go.work"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Skip("TODO: call gowork.Find(dir); assert ws != nil; assert ws.Root == dir; " +
		"assert len(ws.Modules) == 1 and ws.Modules[0] == filepath.Join(dir, 'mod')")
}

// TestGoWork_Find_WalksUpDirectoryTree verifies that Find walks upward from a
// nested subdirectory and finds a go.work file in a parent directory.
func TestGoWork_Find_WalksUpDirectoryTree(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	content := "go 1.21\n\nuse ./mod\n"
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Skip("TODO: call gowork.Find(nested); assert ws.Root == root; " +
		"the search must walk up from a/b/c/ and find the go.work at root/")
}

// TestGoWork_Find_StopsAtFilesystemRoot verifies that Find does not loop
// forever when no go.work exists; it must terminate and return nil.
func TestGoWork_Find_StopsAtFilesystemRoot(t *testing.T) {
	t.Skip("TODO: call gowork.Find('/'); assert result is nil with no deadlock; " +
		"use a timeout or t.Parallel with a context to guard against infinite loops")
}

// TestGoWork_Parse_SingleInlineUse verifies that a go.work with a single
// inline 'use ./path' (not in a block) is parsed correctly.
func TestGoWork_Parse_SingleInlineUse(t *testing.T) {
	root := t.TempDir()
	content := "go 1.21\n\nuse ./mymod\n"
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Skip("TODO: call gowork.Find(root); assert len(ws.Modules)==1 and " +
		"ws.Modules[0] == filepath.Join(root, 'mymod')")
}

// TestGoWork_Parse_MultipleUsesInBlock verifies that a use ( ... ) block
// with multiple paths is fully parsed, producing one Modules entry per path.
func TestGoWork_Parse_MultipleUsesInBlock(t *testing.T) {
	root := t.TempDir()
	content := "go 1.21\n\nuse (\n\t./alpha\n\t./beta\n\t./gamma\n)\n"
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Skip("TODO: call gowork.Find(root); assert len(ws.Modules)==3; " +
		"assert ws.Modules contains filepath.Join(root, 'alpha'), 'beta', 'gamma'")
}

// TestGoWork_Parse_QuotedUsePath verifies that paths written with surrounding
// double quotes (e.g. use \"./mod\") are unquoted correctly.
func TestGoWork_Parse_QuotedUsePath(t *testing.T) {
	root := t.TempDir()
	content := "go 1.21\n\nuse \"./mymod\"\n"
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Skip("TODO: call gowork.Find(root); assert len(ws.Modules)==1 and " +
		"ws.Modules[0] == filepath.Join(root, 'mymod') (no stray quote chars)")
}

// TestGoWork_Parse_InlineCommentStripped verifies that lines with inline //
// comments have the comment portion removed before path parsing.
func TestGoWork_Parse_InlineCommentStripped(t *testing.T) {
	root := t.TempDir()
	content := "go 1.21\n\nuse (\n\t./alpha // primary module\n\t./beta\n)\n"
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Skip("TODO: call gowork.Find(root); assert both modules parsed correctly; " +
		"the '// primary module' comment must not contaminate ws.Modules[0]")
}

// TestGoWork_Parse_BlankLinesAndWhitespace verifies that blank lines and
// leading/trailing whitespace in the use block do not create spurious entries.
func TestGoWork_Parse_BlankLinesAndWhitespace(t *testing.T) {
	root := t.TempDir()
	content := "go 1.21\n\nuse (\n\n\t./alpha\n\n\t./beta\n\n)\n"
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Skip("TODO: call gowork.Find(root); assert exactly 2 modules (alpha, beta); " +
		"no empty-string entries from blank lines")
}

// TestGoWork_Parse_EmptyUseBlock verifies that a use ( ) block with no entries
// produces an empty Modules slice rather than nil or a parse error.
func TestGoWork_Parse_EmptyUseBlock(t *testing.T) {
	root := t.TempDir()
	content := "go 1.21\n\nuse (\n)\n"
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Skip("TODO: call gowork.Find(root); assert ws != nil and len(ws.Modules)==0")
}

// TestGoWork_Parse_NoUseBlock verifies that a go.work with no use block at
// all results in a Workspace with an empty Modules slice (not nil panic).
func TestGoWork_Parse_NoUseBlock(t *testing.T) {
	root := t.TempDir()
	content := "go 1.21\n"
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Skip("TODO: call gowork.Find(root); assert ws != nil, ws.Root == root, len(ws.Modules)==0")
}

// TestGoWork_Parse_AbsoluteUsePath verifies that an absolute path in a use
// entry is preserved as-is (not re-joined with the workspace root).
func TestGoWork_Parse_AbsoluteUsePath(t *testing.T) {
	root := t.TempDir()
	absModule := t.TempDir()
	content := "go 1.21\n\nuse " + absModule + "\n"
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Skip("TODO: call gowork.Find(root); assert ws.Modules[0] == absModule (absolute path unchanged)")
}

// TestGoWork_ContainsPath_FalseForUnrelated verifies that ContainsPath returns
// false for a path that is not inside any workspace member module.
func TestGoWork_ContainsPath_FalseForUnrelated(t *testing.T) {
	ws := &gowork.Workspace{
		Root:    "/workspace",
		Modules: []string{"/workspace/mod"},
	}
	if ws.ContainsPath("/some/other/dir/file.go") {
		t.Error("ContainsPath returned true for path outside all workspace modules")
	}
}

// TestGoWork_ContainsPath_TrueForExactModuleRoot verifies that a path exactly
// equal to a module root directory is considered contained.
func TestGoWork_ContainsPath_TrueForExactModuleRoot(t *testing.T) {
	ws := &gowork.Workspace{
		Root:    "/workspace",
		Modules: []string{"/workspace/mod"},
	}
	if !ws.ContainsPath("/workspace/mod") {
		t.Error("ContainsPath returned false for exact module root path")
	}
}

// TestGoWork_ContainsPath_TrueForNestedPath verifies that a path nested inside
// a module directory is correctly identified as contained.
func TestGoWork_ContainsPath_TrueForNestedPath(t *testing.T) {
	ws := &gowork.Workspace{
		Root:    "/workspace",
		Modules: []string{"/workspace/mod"},
	}
	if !ws.ContainsPath("/workspace/mod/pkg/subpkg/file.go") {
		t.Error("ContainsPath returned false for a path nested inside the module")
	}
}

// TestGoWork_ContainsPath_FalseForSiblingPrefix verifies that a module path
// that is a string prefix of another path but not a directory prefix is not
// matched. e.g. module /mod should NOT match /mod-extra/file.go.
func TestGoWork_ContainsPath_FalseForSiblingPrefix(t *testing.T) {
	ws := &gowork.Workspace{
		Root:    "/workspace",
		Modules: []string{"/workspace/mod"},
	}
	if ws.ContainsPath("/workspace/mod-extra/file.go") {
		t.Error("ContainsPath matched /mod-extra as if it were inside /mod — path separator check missing")
	}
}

// TestGoWork_ContainsPath_MultipleModules_FirstMatch verifies that ContainsPath
// returns true if the path is inside ANY workspace module, not only the first.
func TestGoWork_ContainsPath_MultipleModules_FirstMatch(t *testing.T) {
	ws := &gowork.Workspace{
		Root:    "/workspace",
		Modules: []string{"/workspace/alpha", "/workspace/beta", "/workspace/gamma"},
	}
	if !ws.ContainsPath("/workspace/gamma/pkg/file.go") {
		t.Error("ContainsPath returned false for path in third module")
	}
}

// TestGoWork_ModuleFor_ReturnsEmpty_ForUnrelatedPath verifies that ModuleFor
// returns "" when the path is not inside any workspace member.
func TestGoWork_ModuleFor_ReturnsEmpty_ForUnrelatedPath(t *testing.T) {
	ws := &gowork.Workspace{
		Root:    "/workspace",
		Modules: []string{"/workspace/mod"},
	}
	if got := ws.ModuleFor("/unrelated/path"); got != "" {
		t.Errorf("ModuleFor returned %q for unrelated path, want empty string", got)
	}
}

// TestGoWork_ModuleFor_ReturnsExactMatch verifies that when a path equals a
// module root, ModuleFor returns that exact module root.
func TestGoWork_ModuleFor_ReturnsExactMatch(t *testing.T) {
	ws := &gowork.Workspace{
		Root:    "/workspace",
		Modules: []string{"/workspace/mod"},
	}
	if got := ws.ModuleFor("/workspace/mod"); got != "/workspace/mod" {
		t.Errorf("ModuleFor exact match: want '/workspace/mod' got %q", got)
	}
}

// TestGoWork_ModuleFor_ReturnsLongestMatch verifies that when a path could
// match multiple nested module roots (e.g. /mod and /mod/sub), ModuleFor
// returns the deepest (longest) matching module root.
func TestGoWork_ModuleFor_ReturnsLongestMatch(t *testing.T) {
	ws := &gowork.Workspace{
		Root:    "/workspace",
		Modules: []string{"/workspace/mod", "/workspace/mod/sub"},
	}
	t.Skip("TODO: call ws.ModuleFor('/workspace/mod/sub/pkg/file.go'); " +
		"assert result == '/workspace/mod/sub' (not '/workspace/mod'); " +
		"longest-match wins to correctly attribute files in nested modules")
}

// TestGoWork_ModuleFor_SiblingPrefix_NotMatched verifies that ModuleFor does
// not match a sibling path that shares a string prefix but not a path separator.
func TestGoWork_ModuleFor_SiblingPrefix_NotMatched(t *testing.T) {
	ws := &gowork.Workspace{
		Root:    "/workspace",
		Modules: []string{"/workspace/mod"},
	}
	if got := ws.ModuleFor("/workspace/mod-extra/file.go"); got != "" {
		t.Errorf("ModuleFor matched sibling path as if inside module: got %q", got)
	}
}

// TestGoWork_WorkspaceSetup_CopiesGoWorkToTempDir verifies that when
// ModuleWorkspace.Setup runs and a go.work file exists, the go.work is
// copied into the temp directory so the schemata compilation uses the
// workspace configuration.
func TestGoWork_WorkspaceSetup_CopiesGoWorkToTempDir(t *testing.T) {
	t.Skip("TODO: create a temp dir with a valid go.work referencing a minimal module; " +
		"run NewModuleWorkspace() + ws.Setup(dir, nil); " +
		"assert filepath.Join(ws.TempDir, 'go.work') exists")
}

// TestGoWork_WorkspaceSetup_RewritesUsePaths verifies that copyGoWork rewrites
// each 'use' path in the copied go.work to be relative to the temp directory,
// not the original workspace root. Without this, the copied workspace would
// still reference the original source paths.
func TestGoWork_WorkspaceSetup_RewritesUsePaths(t *testing.T) {
	t.Skip("TODO: create a workspace with go.work 'use ./modA'; run ws.Setup; " +
		"read the copied go.work in TempDir; assert the use path points inside TempDir, " +
		"not back to the original directory")
}

// TestGoWork_WorkspaceSetup_CopiesGoWorkSum verifies that go.work.sum is
// copied alongside go.work when it exists, so the build in the temp workspace
// can resolve dependencies without a network fetch.
func TestGoWork_WorkspaceSetup_CopiesGoWorkSum(t *testing.T) {
	t.Skip("TODO: create a workspace with both go.work and go.work.sum; run ws.Setup; " +
		"assert filepath.Join(ws.TempDir, 'go.work.sum') exists with identical content")
}

// TestGoWork_WorkspaceSetup_FallsBackToGoMod_WhenNoGoWork verifies that
// ModuleWorkspace.Setup handles a plain module project (no go.work) correctly
// by discovering go.mod and treating it as the sole module root.
func TestGoWork_WorkspaceSetup_FallsBackToGoMod_WhenNoGoWork(t *testing.T) {
	repoRoot := findRepoRoot(t)
	t.Skip("TODO: verify that ws.Setup(repoRoot, nil) succeeds even when the repo has " +
		"no go.work (single-module mode); assert ws.TempDir contains go.mod")
}

// TestGoWork_WorkspaceSetup_MultiModule_AllModulesCopied verifies that when
// a go.work references multiple member modules, Setup copies all of them into
// the temp workspace, not just the one containing mutated files.
func TestGoWork_WorkspaceSetup_MultiModule_AllModulesCopied(t *testing.T) {
	t.Skip("TODO: create a workspace with two member modules each containing a .go file; " +
		"run ws.Setup; assert .go files from BOTH modules appear under ws.TempDir; " +
		"a missing module would cause 'package not found' errors during schemata compilation")
}

// TestGoWork_WorkspaceSetup_MutatedFileOnly_SkipsUnmutatedPackages verifies
// that Setup does not copy every file in the project — only the packages that
// contain mutated sites, plus their dependencies. Large projects benefit from
// this: copying everything would be slow.
func TestGoWork_WorkspaceSetup_MutatedFileOnly_SkipsUnmutatedPackages(t *testing.T) {
	t.Skip("TODO: create a workspace with two unrelated packages A and B; " +
		"set mutatedPaths to only files in A; run ws.Setup; " +
		"assert B's files do NOT appear in ws.TempDir unless they are transitive deps")
}

// TestGoWork_RelPath_UsesModuleRootForFile verifies that relPath computes
// the path of a file relative to its owning module root in multi-module mode,
// not relative to the workspace root. This ensures transformed files are
// placed at the correct path inside the temp directory.
func TestGoWork_RelPath_UsesModuleRootForFile(t *testing.T) {
	t.Skip("TODO: construct a ModuleWorkspace with goWork set to a two-member workspace; " +
		"call relPath for a file in the second member module; " +
		"assert the returned path is relative to the workspace root (not module root) " +
		"so it lands at the correct location inside TempDir")
}

// TestGoWork_RelPath_RejectsFileOutsideWorkspace verifies that relPath returns
// an error when the file path is outside all known workspace roots, preventing
// a path traversal (\"../\"-escaped) write into the temp directory.
func TestGoWork_RelPath_RejectsFileOutsideWorkspace(t *testing.T) {
	t.Skip("TODO: construct a ModuleWorkspace; call relPath with a path that resolves " +
		"to '../outside/the/workspace'; assert an error is returned (not a panic or " +
		"silent write outside TempDir)")
}

// TestGoWork_Find_ParseError_ReturnsNil verifies that if go.work exists but
// cannot be read (e.g. permission denied), Find returns nil rather than
// propagating the error. Callers treat nil as "no workspace" and fall back
// to go.mod.
func TestGoWork_Find_ParseError_ReturnsNil(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — permission restrictions do not apply")
	}
	root := t.TempDir()
	p := filepath.Join(root, "go.work")
	if err := os.WriteFile(p, []byte("go 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(p, 0644) })

	t.Skip("TODO: call gowork.Find(root) after making go.work unreadable; " +
		"assert nil is returned (not a panic or error propagation)")
}

// TestGoWork_Discover_AndSubconfig_Cooperation verifies that subconfig.Discover
// in a workspace correctly walks all workspace member directories, not just the
// directory containing go.work. Sub-configs in member modules must be found.
func TestGoWork_Discover_AndSubconfig_Cooperation(t *testing.T) {
	t.Skip("TODO: create a workspace with two member modules; place a gorgon.yml in " +
		"member module B; call subconfig.Discover with the workspace root; " +
		"assert the sub-config from module B is in the resolver's entries")
}
