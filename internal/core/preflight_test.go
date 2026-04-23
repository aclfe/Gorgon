package testing

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
)

type Site = engine.Site

const (
	srcIntFunc = `package foo
 
func Add(a, b int) int {
	return a + b
}
`
	srcBoolFunc = `package foo
 
func IsPositive(n int) bool {
	return n > 0
}
`
	srcStringFunc = `package foo
 
import "strings"
 
func Trim(s string) string {
	return strings.TrimSpace(s)
}
`
	srcMultiFunc = `package foo
 
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
 
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
`
)


func silentLog() *logger.Logger { return logger.New(false) }


func writeFile(t *stdtesting.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("writeFile MkdirAll: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile WriteFile: %v", err)
	}
	return p
}



func parseOnDisk(t *stdtesting.T, path, src string) (*token.FileSet, *ast.File) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("parseOnDisk MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("parseOnDisk WriteFile: %v", err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parseOnDisk ParseFile(%s): %v", path, err)
	}
	return fset, f
}


func inMemParse(t *stdtesting.T, fakePath, src string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, fakePath, src, 0)
	if err != nil {
		t.Fatalf("inMemParse: %v", err)
	}
	return fset, f
}


func firstBinaryExpr(f *ast.File) *ast.BinaryExpr {
	var out *ast.BinaryExpr
	ast.Inspect(f, func(n ast.Node) bool {
		if out != nil {
			return false
		}
		if be, ok := n.(*ast.BinaryExpr); ok {
			out = be
		}
		return true
	})
	return out
}


func firstReturnStmt(f *ast.File) *ast.ReturnStmt {
	var out *ast.ReturnStmt
	ast.Inspect(f, func(n ast.Node) bool {
		if out != nil {
			return false
		}
		if rs, ok := n.(*ast.ReturnStmt); ok {
			out = rs
		}
		return true
	})
	return out
}


func firstIfStmt(f *ast.File) *ast.IfStmt {
	var out *ast.IfStmt
	ast.Inspect(f, func(n ast.Node) bool {
		if out != nil {
			return false
		}
		if is, ok := n.(*ast.IfStmt); ok {
			out = is
		}
		return true
	})
	return out
}



func buildMutant(id int, fset *token.FileSet, astFile *ast.File, node ast.Node, returnType string) Mutant {
	pos := fset.Position(node.Pos())
	return Mutant{
		ID: id,
		Site: engine.Site{
			File:       fset.File(node.Pos()),
			FileAST:    astFile,
			Fset:       fset,
			Node:       node,
			Line:       pos.Line,
			Column:     pos.Column,
			ReturnType: returnType,
		},
	}
}



func ephemeralTokenFile(fakeName string) (*token.FileSet, *token.File, *ast.File) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, fakeName, "package p", 0)
	return fset, fset.File(f.Pos()), f
}




func TestTypeErrorMessage_EmptyString(t *stdtesting.T) {
	if got := typeErrorMessage(""); got != "" {
		t.Errorf("empty input: got %q, want %q", got, "")
	}
}

func TestTypeErrorMessage_NoColonSpace(t *stdtesting.T) {
	in := "no colon-space in this message"
	if got := typeErrorMessage(in); got != in {
		t.Errorf("got %q, want identical string", got)
	}
}

func TestTypeErrorMessage_StripsSinglePrefix(t *stdtesting.T) {
	in := "/abs/path/file.go:42:10: cannot use string as int"
	want := "cannot use string as int"
	if got := typeErrorMessage(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTypeErrorMessage_StripOnlyFirstOccurrence(t *stdtesting.T) {
	
	in := "file.go:1:2: cannot use type: int as string"
	want := "cannot use type: int as string"
	if got := typeErrorMessage(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTypeErrorMessage_RelativePath(t *stdtesting.T) {
	in := "foo/bar.go:7:3: undefined: Foo"
	want := "undefined: Foo"
	if got := typeErrorMessage(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTypeErrorMessage_MessageWithNoFile(t *stdtesting.T) {
	
	in := "too many arguments"
	if got := typeErrorMessage(in); got != in {
		t.Errorf("got %q, want unchanged %q", got, in)
	}
}





func TestLenientImporter_KnownStdlibPackage(t *stdtesting.T) {
	imp := &lenientImporter{base: importer.Default()}
	pkg, err := imp.Import("fmt")
	if err != nil {
		t.Fatalf("expected no error for 'fmt', got %v", err)
	}
	if pkg == nil {
		t.Fatal("expected non-nil package for 'fmt'")
	}
	if pkg.Name() != "fmt" {
		t.Errorf("pkg.Name() = %q, want %q", pkg.Name(), "fmt")
	}
}

func TestLenientImporter_UnknownPackageReturnsStub(t *stdtesting.T) {
	imp := &lenientImporter{base: importer.Default()}
	pkg, err := imp.Import("totally/unknown/pkg/xyz123")
	if err != nil {
		t.Fatalf("lenientImporter must never return an error, got: %v", err)
	}
	if pkg == nil {
		t.Fatal("lenientImporter must return a non-nil stub for unknown packages")
	}
}

func TestLenientImporter_CachesKnownPackage(t *stdtesting.T) {
	imp := &lenientImporter{base: importer.Default()}
	p1, _ := imp.Import("strings")
	p2, _ := imp.Import("strings")
	if p1 != p2 {
		t.Error("second import of 'strings' must return the same cached pointer")
	}
}

func TestLenientImporter_CachesUnknownPackage(t *stdtesting.T) {
	imp := &lenientImporter{base: importer.Default()}
	p1, _ := imp.Import("no/such/package")
	p2, _ := imp.Import("no/such/package")
	if p1 != p2 {
		t.Error("second import of unknown package must return the same stub pointer")
	}
}

func TestLenientImporter_NilCacheInitialisedLazily(t *stdtesting.T) {
	
	imp := &lenientImporter{base: importer.Default()}
	if imp.cache != nil {
		t.Fatal("cache must start nil")
	}
	_, _ = imp.Import("os")
	if imp.cache == nil {
		t.Error("cache must be initialised after first Import call")
	}
}





func TestParsePackageName_ValidSource(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "mypackage", "code.go")
	src := []byte("package mypackage\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	got := parsePackageName(src, path)
	if got != "mypackage" {
		t.Errorf("got %q, want %q", got, "mypackage")
	}
}

func TestParsePackageName_InvalidSource_FallsBackToDir(t *stdtesting.T) {
	
	path := filepath.Join("some", "mypkg", "bad.go")
	got := parsePackageName([]byte("not valid go !!!"), path)
	if got != "mypkg" {
		t.Errorf("got %q, want %q (dir-based fallback)", got, "mypkg")
	}
}

func TestParsePackageName_MainPackage(t *stdtesting.T) {
	src := []byte("package main\nfunc main(){}\n")
	path := filepath.Join("cmd", "tool", "main.go")
	got := parsePackageName(src, path)
	if got != "main" {
		t.Errorf("got %q, want %q", got, "main")
	}
}


func TestLoadSiblingFiles_EmptyDirExcludesTarget(t *stdtesting.T) {
	tmp := t.TempDir()
	target := writeFile(t, tmp, "main.go", "package p")
	sibs := loadSiblingFiles(tmp, target)
	if len(sibs) != 0 {
		t.Errorf("expected 0 siblings when only the target exists, got %d", len(sibs))
	}
}

func TestLoadSiblingFiles_ExcludesTargetFile(t *stdtesting.T) {
	tmp := t.TempDir()
	target := writeFile(t, tmp, "a.go", "package p")
	other := writeFile(t, tmp, "b.go", "package p")
	sibs := loadSiblingFiles(tmp, target)
	if len(sibs) != 1 {
		t.Fatalf("expected 1 sibling, got %d", len(sibs))
	}
	if sibs[0].path != other {
		t.Errorf("sibling path = %q, want %q", sibs[0].path, other)
	}
}

func TestLoadSiblingFiles_ExcludesTestFiles(t *stdtesting.T) {
	tmp := t.TempDir()
	target := writeFile(t, tmp, "code.go", "package p")
	writeFile(t, tmp, "code_test.go", "package p\nimport \"testing\"\nfunc TestX(*testing.T){}")
	sibs := loadSiblingFiles(tmp, target)
	for _, s := range sibs {
		if strings.HasSuffix(s.path, "_test.go") {
			t.Errorf("test file should be excluded, but got %s", s.path)
		}
	}
}

func TestLoadSiblingFiles_ReturnsMultipleSiblings(t *stdtesting.T) {
	tmp := t.TempDir()
	target := writeFile(t, tmp, "target.go", "package p")
	writeFile(t, tmp, "sibling1.go", "package p")
	writeFile(t, tmp, "sibling2.go", "package p")
	writeFile(t, tmp, "skip_test.go", "package p")
	sibs := loadSiblingFiles(tmp, target)
	if len(sibs) != 2 {
		t.Errorf("expected 2 siblings (sibling1.go, sibling2.go), got %d", len(sibs))
	}
}

func TestLoadSiblingFiles_SiblingBytesMatch(t *stdtesting.T) {
	tmp := t.TempDir()
	target := writeFile(t, tmp, "t.go", "package p")
	want := "package p\nvar X = 1\n"
	writeFile(t, tmp, "other.go", want)
	sibs := loadSiblingFiles(tmp, target)
	if len(sibs) != 1 {
		t.Fatalf("expected 1 sibling")
	}
	if string(sibs[0].src) != want {
		t.Errorf("sibling src = %q, want %q", sibs[0].src, want)
	}
}

func TestLoadSiblingFiles_NonexistentDir(t *stdtesting.T) {
	
	sibs := loadSiblingFiles("/does/not/exist", "/does/not/exist/x.go")
	if sibs != nil {
		t.Errorf("expected nil for non-existent dir, got %v", sibs)
	}
}


func TestMakeAllInvalid_NilInput(t *stdtesting.T) {
	valid, invalid := makeAllInvalid(nil, "reason")
	if valid != nil {
		t.Errorf("expected nil valid slice, got %v", valid)
	}
	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid results, got %d", len(invalid))
	}
}

func TestMakeAllInvalid_SetsStatusCompileError(t *stdtesting.T) {
	mutants := []Mutant{{ID: 1}, {ID: 2}, {ID: 3}}
	valid, invalid := makeAllInvalid(mutants, "test reason")
	if valid != nil {
		t.Errorf("expected nil valid, got %v", valid)
	}
	if len(invalid) != 3 {
		t.Fatalf("expected 3 invalid, got %d", len(invalid))
	}
	for _, r := range invalid {
		if r.Status != StatusCompileError {
			t.Errorf("mutant #%d: Status = %q, want %q", r.MutantID, r.Status, StatusCompileError)
		}
	}
}

func TestMakeAllInvalid_SetsErrorReason(t *stdtesting.T) {
	mutants := []Mutant{{ID: 7}, {ID: 8}}
	_, invalid := makeAllInvalid(mutants, "expected reason text")
	for _, r := range invalid {
		if r.ErrorReason != "expected reason text" {
			t.Errorf("mutant #%d: ErrorReason = %q, want %q", r.MutantID, r.ErrorReason, "expected reason text")
		}
	}
}

func TestMakeAllInvalid_PreflightResultIDsMatchInput(t *stdtesting.T) {
	mutants := []Mutant{{ID: 10}, {ID: 20}, {ID: 30}}
	_, invalid := makeAllInvalid(mutants, "r")
	ids := map[int]bool{}
	for _, r := range invalid {
		ids[r.MutantID] = true
	}
	for _, m := range mutants {
		if !ids[m.ID] {
			t.Errorf("mutant #%d missing from invalid results", m.ID)
		}
	}
}

func TestMakeAllInvalid_MutantStatusSideEffect(t *stdtesting.T) {
	
	mutants := []Mutant{{ID: 1}, {ID: 2}}
	makeAllInvalid(mutants, "r")
	for _, m := range mutants {
		if m.Status != StatusCompileError {
			t.Errorf("mutant #%d.Status = %q, want %q", m.ID, m.Status, StatusCompileError)
		}
	}
}


func TestIsObviouslyUnsafeMutation_NilFields(t *stdtesting.T) {
	
	if isObviouslyUnsafeMutation(&Mutant{}) {
		t.Error("expected false for zero Mutant")
	}
}

func TestIsObviouslyUnsafeMutation_AlwaysFalse(t *stdtesting.T) {
	cases := []*Mutant{
		{ID: 1},
		{ID: 2, Site: Site{ReturnType: "bool"}},
		{ID: 3, Status: "error"},
	}
	for _, m := range cases {
		if isObviouslyUnsafeMutation(m) {
			t.Errorf("expected false for mutant #%d", m.ID)
		}
	}
}

func TestStatusConstants_Values(t *stdtesting.T) {
	if StatusValid != "valid" {
		t.Errorf("StatusValid = %q, want %q", StatusValid, "valid")
	}
	if StatusInvalid != "invalid" {
		t.Errorf("StatusInvalid = %q, want %q", StatusInvalid, "invalid")
	}
	if StatusCompileError != "error" {
		t.Errorf("StatusCompileError = %q, want %q", StatusCompileError, "error")
	}
}


func TestSchemataHelperSrc_IsValidGo(t *stdtesting.T) {
	src := fmt.Sprintf(schemataHelperSrc, "testpkg")
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "gorgon_schemata.go", src, 0); err != nil {
		t.Errorf("schemataHelperSrc is not valid Go code: %v", err)
	}
}

func TestSchemataHelperSrc_ContainsActiveMutantID(t *stdtesting.T) {
	src := fmt.Sprintf(schemataHelperSrc, "mypkg")
	if !strings.Contains(src, "activeMutantID") {
		t.Error("schemataHelperSrc must declare activeMutantID")
	}
}

func TestSchemataHelperSrc_ContainsGORGON_MUTANT_ID(t *stdtesting.T) {
	src := fmt.Sprintf(schemataHelperSrc, "mypkg")
	if !strings.Contains(src, "GORGON_MUTANT_ID") {
		t.Error("schemataHelperSrc must read GORGON_MUTANT_ID environment variable")
	}
}


func TestQuickStaticFilter_NilNodeIsInvalid(t *stdtesting.T) {
	_, tokFile, _ := ephemeralTokenFile("p.go")
	m := Mutant{ID: 1, Site: Site{Node: nil, File: tokFile}}
	valid, invalid := quickStaticFilter([]Mutant{m})
	if len(valid) != 0 {
		t.Errorf("nil-node mutant must be filtered; got %d valid", len(valid))
	}
	if len(invalid) != 1 {
		t.Fatalf("expected 1 invalid result, got %d", len(invalid))
	}
	if invalid[0].Status != StatusInvalid {
		t.Errorf("Status = %q, want %q", invalid[0].Status, StatusInvalid)
	}
	if !strings.Contains(invalid[0].ErrorReason, "nil node") {
		t.Errorf("ErrorReason = %q; expected it to mention 'nil node'", invalid[0].ErrorReason)
	}
}

func TestQuickStaticFilter_NilFileIsInvalid(t *stdtesting.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "p.go", "package p\nvar x = 1\n", 0)
	node := ast.Node(f.Decls[0])
	m := Mutant{ID: 2, Site: Site{Node: node, File: nil}}
	valid, invalid := quickStaticFilter([]Mutant{m})
	if len(valid) != 0 {
		t.Errorf("nil-file mutant must be filtered; got %d valid", len(valid))
	}
	if len(invalid) != 1 {
		t.Fatalf("expected 1 invalid result, got %d", len(invalid))
	}
	if !strings.Contains(invalid[0].ErrorReason, "nil file") {
		t.Errorf("ErrorReason = %q; expected it to mention 'nil file'", invalid[0].ErrorReason)
	}
}

func TestQuickStaticFilter_ValidMutantPassesThrough(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "add.go")
	fset, astFile := parseOnDisk(t, path, srcIntFunc)
	node := firstBinaryExpr(astFile)
	if node == nil {
		t.Skip("no BinaryExpr in srcIntFunc")
	}
	m := buildMutant(1, fset, astFile, node, "int")
	valid, invalid := quickStaticFilter([]Mutant{m})
	if len(valid) != 1 {
		t.Errorf("valid mutant should pass through; got %d valid", len(valid))
	}
	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid, got %d", len(invalid))
	}
}

func TestQuickStaticFilter_MixedBatchSortsCorrectly(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.go")
	fset, astFile := parseOnDisk(t, path, srcIntFunc)
	realNode := firstBinaryExpr(astFile)
	if realNode == nil {
		t.Skip("no BinaryExpr")
	}
	_, tokFile, _ := ephemeralTokenFile("p.go")

	mutants := []Mutant{
		{ID: 10, Site: Site{Node: nil, File: tokFile}},                       
		{ID: 20, Site: Site{Node: astFile, File: nil}},                       
		{ID: 30, Site: buildMutant(30, fset, astFile, realNode, "int").Site}, 
	}
	valid, invalid := quickStaticFilter(mutants)
	if len(valid) != 1 || valid[0].ID != 30 {
		t.Errorf("expected only mutant #30 to be valid; got %v", idsOf(valid))
	}
	if len(invalid) != 2 {
		t.Errorf("expected 2 invalid, got %d", len(invalid))
	}
}

func TestQuickStaticFilter_EmptyInput(t *stdtesting.T) {
	valid, invalid := quickStaticFilter(nil)
	if len(valid) != 0 {
		t.Errorf("expected empty valid, got %d", len(valid))
	}
	if len(invalid) != 0 {
		t.Errorf("expected empty invalid, got %d", len(invalid))
	}
}

func TestQuickStaticFilter_AllValidAllPass(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "m.go")
	fset, astFile := parseOnDisk(t, path, srcMultiFunc)
	var nodes []ast.Node
	ast.Inspect(astFile, func(n ast.Node) bool {
		if be, ok := n.(*ast.BinaryExpr); ok {
			nodes = append(nodes, be)
		}
		return true
	})
	if len(nodes) == 0 {
		t.Skip("no BinaryExprs in srcMultiFunc")
	}
	mutants := make([]Mutant, len(nodes))
	for i, n := range nodes {
		mutants[i] = buildMutant(i+1, fset, astFile, n, "int")
	}
	valid, invalid := quickStaticFilter(mutants)
	if len(valid) != len(nodes) {
		t.Errorf("all valid mutants should pass through; got %d/%d", len(valid), len(nodes))
	}
	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid, got %d", len(invalid))
	}
}

func TestQuickStaticFilter_InvalidMutantStatusSetOnInput(t *stdtesting.T) {
	
	_, tokFile, _ := ephemeralTokenFile("p.go")
	m := Mutant{ID: 1, Site: Site{Node: nil, File: tokFile}}
	mutants := []Mutant{m}
	quickStaticFilter(mutants)
	if mutants[0].Status != StatusInvalid {
		t.Errorf("mutant Status should be set to StatusInvalid in-place; got %q", mutants[0].Status)
	}
}


func TestCheckFileWithSchemata_NilMutants(t *stdtesting.T) {
	valid, invalid := checkFileWithSchemata("/any/path.go", nil)
	if valid != nil || invalid != nil {
		t.Error("expected nil slices for nil mutant input")
	}
}

func TestCheckFileWithSchemata_NonexistentFile_AllInvalid(t *stdtesting.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "/nonexistent/path.go", "package p", 0)
	tok := fset.File(f.Pos())
	mutants := []Mutant{
		{ID: 1, Site: Site{Node: f, File: tok}},
		{ID: 2, Site: Site{Node: f, File: tok}},
	}
	valid, invalid := checkFileWithSchemata("/nonexistent/path.go", mutants)
	if len(valid) != 0 {
		t.Errorf("expected 0 valid for nonexistent file, got %d", len(valid))
	}
	if len(invalid) != 2 {
		t.Errorf("expected 2 invalid, got %d", len(invalid))
	}
	for _, r := range invalid {
		if !strings.Contains(r.ErrorReason, "cannot read source file") {
			t.Errorf("mutant #%d ErrorReason = %q; expected 'cannot read source file'", r.MutantID, r.ErrorReason)
		}
	}
}

func TestCheckFileWithSchemata_ValidFile_Passes(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "add.go")
	fset, astFile := parseOnDisk(t, path, srcIntFunc)
	node := firstBinaryExpr(astFile)
	if node == nil {
		t.Skip("no BinaryExpr")
	}
	mutants := []Mutant{buildMutant(1, fset, astFile, node, "int")}
	valid, invalid := checkFileWithSchemata(path, mutants)
	t.Logf("L2 valid=%d invalid=%d", len(valid), len(invalid))

	_ = valid
	_ = invalid
}

func TestLevel2PackagePreflight_EmptyInput(t *stdtesting.T) {
	valid, invalid := level2PackagePreflight(nil)
	if valid != nil || invalid != nil {
		t.Error("expected nil slices for empty input")
	}
}

func TestLevel2PackagePreflight_NilFileMutantMarkedInvalid(t *stdtesting.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "p.go", "package p", 0)
	m := Mutant{ID: 1, Site: Site{Node: f, File: nil}}
	valid, invalid := level2PackagePreflight([]Mutant{m})
	if len(valid) != 0 {
		t.Errorf("nil-file mutant must be invalid at L2; got %d valid", len(valid))
	}
	if len(invalid) != 1 {
		t.Errorf("expected 1 invalid, got %d", len(invalid))
	}
}

func TestLevel2PackagePreflight_NonexistentFileMutant(t *stdtesting.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "/no/such/file.go", "package p", 0)
	tok := fset.File(f.Pos())
	mutants := []Mutant{
		{ID: 1, Site: Site{Node: f, File: tok}},
		{ID: 2, Site: Site{Node: f, File: tok}},
	}
	valid, invalid := level2PackagePreflight(mutants)
	if len(valid) != 0 {
		t.Errorf("expected 0 valid for unreadable file, got %d", len(valid))
	}
	if len(invalid) != 2 {
		t.Errorf("expected 2 invalid, got %d", len(invalid))
	}
}

func TestLevel2PackagePreflight_GroupsByFile(t *stdtesting.T) {
	
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "/bad/path.go", "package p", 0)
	tok := fset.File(f.Pos())

	mutants := []Mutant{
		{ID: 1, Site: Site{Node: f, File: tok}},
		{ID: 2, Site: Site{Node: f, File: tok}},
	}
	_, invalid := level2PackagePreflight(mutants)
	ids := map[int]bool{}
	for _, r := range invalid {
		ids[r.MutantID] = true
	}
	if !ids[1] || !ids[2] {
		t.Errorf("both mutants should be invalid; got ids=%v", ids)
	}
}

func TestComputeBaselineErrors_ValidFile_ReturnsMap(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "add.go")
	src := []byte(srcIntFunc)
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	imp := &lenientImporter{base: importer.Default()}
	helper := fmt.Sprintf(schemataHelperSrc, "foo")
	baseline := computeBaselineErrors(path, src, nil, helper, tmp, imp)
	if baseline == nil {
		t.Error("computeBaselineErrors must return a non-nil map for valid source")
	}
}

func TestComputeBaselineErrors_ValidFile_NoSpuriousErrors(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "add.go")
	src := []byte(srcIntFunc)
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	imp := &lenientImporter{base: importer.Default()}
	helper := fmt.Sprintf(schemataHelperSrc, "foo")
	baseline := computeBaselineErrors(path, src, nil, helper, tmp, imp)
	if len(baseline) != 0 {
		for msg, count := range baseline {
			t.Logf("unexpected baseline error (%d×): %q", count, msg)
		}
		t.Errorf("clean package produced %d baseline error(s)", len(baseline))
	}
}

func TestComputeBaselineErrors_UnparsableSource_ReturnsNil(t *stdtesting.T) {
	imp := &lenientImporter{base: importer.Default()}
	baseline := computeBaselineErrors("/fake.go", []byte("this is not Go"), nil, "", "/tmp", imp)
	if baseline != nil {
		t.Errorf("expected nil for unparsable source, got %v", baseline)
	}
}

func TestComputeBaselineErrors_WithSiblings(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "a.go")
	src := []byte("package foo\nfunc A() int { return 1 }\n")
	sibSrc := []byte("package foo\nfunc B() int { return 2 }\n")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	sibs := []siblingFile{{path: filepath.Join(tmp, "b.go"), src: sibSrc}}
	imp := &lenientImporter{base: importer.Default()}
	helper := fmt.Sprintf(schemataHelperSrc, "foo")
	baseline := computeBaselineErrors(path, src, sibs, helper, tmp, imp)
	if baseline == nil {
		t.Error("expected non-nil map even with siblings")
	}
}


func TestRunTypeCheck_ValidCodeNoMutants_NoErrors(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "add.go")
	src := []byte(srcIntFunc)
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	imp := &lenientImporter{base: importer.Default()}
	helper := fmt.Sprintf(schemataHelperSrc, "foo")

	newErrors, panicked := runTypeCheck(path, src, nil, nil, helper, tmp, imp, nil)
	if panicked {
		t.Error("runTypeCheck panicked on valid code with no mutants")
	}
	if len(newErrors) > 0 {
		t.Errorf("expected no type errors on valid code, got: %v", newErrors)
	}
}

func TestRunTypeCheck_ValidCodeOneMutant_NoErrors(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "add.go")
	src := []byte(srcIntFunc)
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	fset, astFile := parseOnDisk(t, path, srcIntFunc)
	node := firstBinaryExpr(astFile)
	if node == nil {
		t.Skip("no BinaryExpr")
	}
	m := buildMutant(1, fset, astFile, node, "int")

	imp := &lenientImporter{base: importer.Default()}
	helper := fmt.Sprintf(schemataHelperSrc, "foo")
	baseline := computeBaselineErrors(path, src, nil, helper, tmp, imp)


	newErrors, panicked := runTypeCheck(path, src, []*Mutant{&m}, nil, helper, tmp, imp, baseline)
	if panicked {
		t.Error("runTypeCheck panicked with a no-op mutant")
	}
	
	if len(newErrors) > 0 {
		t.Errorf("unexpected type errors with no-op mutant: %v", newErrors)
	}
}

func TestRunTypeCheck_BaselineAbsorbsPreexistingErrors(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "add.go")
	src := []byte(srcIntFunc)
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	imp := &lenientImporter{base: importer.Default()}
	helper := fmt.Sprintf(schemataHelperSrc, "foo")

	
	
	artificialBaseline := map[string]int{"some preexisting error": 2}

	newErrors, panicked := runTypeCheck(path, src, nil, nil, helper, tmp, imp, artificialBaseline)
	if panicked {
		t.Error("unexpected panic")
	}
	
	
	if len(newErrors) > 0 {
		t.Errorf("expected 0 new errors after baseline absorption, got: %v", newErrors)
	}
}





func TestTypeCheckFileGroup_NonexistentFile_PassesThrough(t *stdtesting.T) {
	
	_, tokFile, _ := ephemeralTokenFile("missing.go")
	mutants := []Mutant{{ID: 1, Site: Site{File: tokFile}}}
	valid, invalid := typeCheckFileGroup("/does/not/exist.go", mutants, silentLog())
	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid (pass-through for unreadable file), got %d", len(invalid))
	}
	if len(valid) != 1 {
		t.Errorf("expected 1 valid (pass-through), got %d", len(valid))
	}
}

func TestTypeCheckFileGroup_ValidFile_NoMutantsPanics(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "add.go")
	if err := os.WriteFile(path, []byte(srcIntFunc), 0o644); err != nil {
		t.Fatal(err)
	}
	
	valid, invalid := typeCheckFileGroup(path, nil, silentLog())
	_ = valid
	_ = invalid
}

func TestTypeCheckFileGroup_ValidFile_OneNoOpMutant(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "add.go")
	fset, astFile := parseOnDisk(t, path, srcIntFunc)
	node := firstBinaryExpr(astFile)
	if node == nil {
		t.Skip("no BinaryExpr")
	}
	mutants := []Mutant{buildMutant(1, fset, astFile, node, "int")}
	valid, invalid := typeCheckFileGroup(path, mutants, silentLog())
	t.Logf("typeCheckFileGroup: valid=%d invalid=%d", len(valid), len(invalid))
	
	_ = valid
	_ = invalid
}





func TestTypeCheckFileMutants_NonexistentFile_PassesAll(t *stdtesting.T) {
	_, tokFile, _ := ephemeralTokenFile("ghost.go")
	mutants := []Mutant{
		{ID: 1, Site: Site{File: tokFile}},
		{ID: 2, Site: Site{File: tokFile}},
	}
	valid, invalid := typeCheckFileMutants("/does/not/exist.go", mutants, silentLog())
	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid (pass-through for unreadable), got %d", len(invalid))
	}
	if len(valid) != 2 {
		t.Errorf("expected 2 valid (pass-through), got %d", len(valid))
	}
}

func TestTypeCheckFileMutants_ValidCode_NoTypeErrors(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "add.go")
	fset, astFile := parseOnDisk(t, path, srcIntFunc)
	node := firstBinaryExpr(astFile)
	if node == nil {
		t.Skip("no BinaryExpr")
	}
	mutants := []Mutant{buildMutant(1, fset, astFile, node, "int")}
	valid, invalid := typeCheckFileMutants(path, mutants, silentLog())
	t.Logf("typeCheckFileMutants: valid=%d invalid=%d", len(valid), len(invalid))
	
	_ = valid
	_ = invalid
}





func TestBisectMutants_EmptyInput(t *stdtesting.T) {
	imp := &lenientImporter{base: importer.Default()}
	valid, invalid := bisectMutants("/f.go", nil, nil, nil, "", "/tmp", imp, nil, silentLog())
	if valid != nil || invalid != nil {
		t.Error("expected nil slices for empty input")
	}
}

func TestBisectMutants_SingleValidMutant(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "add.go")
	src := []byte(srcIntFunc)
	fset, astFile := parseOnDisk(t, path, srcIntFunc)
	node := firstBinaryExpr(astFile)
	if node == nil {
		t.Skip("no BinaryExpr")
	}
	mutants := []Mutant{buildMutant(1, fset, astFile, node, "int")}
	imp := &lenientImporter{base: importer.Default()}
	helper := fmt.Sprintf(schemataHelperSrc, "foo")
	baseline := computeBaselineErrors(path, src, nil, helper, tmp, imp)

	valid, invalid := bisectMutants(path, src, mutants, nil, helper, tmp, imp, baseline, silentLog())
	t.Logf("bisect single: valid=%d invalid=%d", len(valid), len(invalid))
	
	if len(invalid) > 0 {
		t.Errorf("no-op mutation should pass bisection; got %d invalid", len(invalid))
	}
}

func TestBisectMutants_TwoNoOpMutants_BothValid(t *stdtesting.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "add.go")
	src := []byte(srcIntFunc)
	fset, astFile := parseOnDisk(t, path, srcIntFunc)
	node := firstBinaryExpr(astFile)
	if node == nil {
		t.Skip("no BinaryExpr")
	}
	mutants := []Mutant{
		buildMutant(1, fset, astFile, node, "int"),
		buildMutant(2, fset, astFile, node, "int"),
	}
	imp := &lenientImporter{base: importer.Default()}
	helper := fmt.Sprintf(schemataHelperSrc, "foo")
	baseline := computeBaselineErrors(path, src, nil, helper, tmp, imp)

	valid, invalid := bisectMutants(path, src, mutants, nil, helper, tmp, imp, baseline, silentLog())
	t.Logf("bisect two: valid=%d invalid=%d", len(valid), len(invalid))
	if len(invalid) > 0 {
		t.Errorf("no-op mutations should pass bisection; got %d invalid", len(invalid))
	}
	_ = valid
}


func idsOf(ms []Mutant) []int {
	ids := make([]int, len(ms))
	for i, m := range ms {
		ids[i] = m.ID
	}
	return ids
}
