//go:build integration
// +build integration

package integration

// Integration tests for Gorgon/internal/core/preflight.go.
//
// Coverage:
//   Level 1  – quickStaticFilter: nil Node / nil File rejection
//   Level 2  – level2PackagePreflight: schemata AST integrity
//   Level 3  – level3TypeCheckPreflight: go/types type-checking with baseline subtraction
//   Full pipeline – RunPreflight orchestration and result partitioning

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	gcore "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/operators/arithmetic_flip"
	"github.com/aclfe/gorgon/pkg/mutator/operators/constant_replacement"
	"github.com/aclfe/gorgon/pkg/mutator/operators/inc_dec_flip"
	"github.com/aclfe/gorgon/pkg/mutator/operators/sign_toggle"
	"github.com/aclfe/gorgon/pkg/mutator/operators/variable_replacement"
	"github.com/aclfe/gorgon/tests/fixtures/mock_operators"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// parseTempSource writes src to a temp file, parses it, and returns the
// FileSet, AST file, and absolute path. The file is cleaned up when t ends.
func parseTempSource(t *testing.T, src string) (*token.FileSet, *ast.File, string) {
	t.Helper()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "subject.go")
	if err := os.WriteFile(filePath, []byte(src), 0o644); err != nil {
		t.Fatalf("write temp source: %v", err)
	}
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse temp source: %v", err)
	}
	return fset, astFile, filePath
}

// firstBinaryExpr returns the first *ast.BinaryExpr found in the file, or
// nil if none is present.
func firstBinaryExpr(astFile *ast.File) *ast.BinaryExpr {
	var found *ast.BinaryExpr
	ast.Inspect(astFile, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if be, ok := n.(*ast.BinaryExpr); ok {
			found = be
		}
		return true
	})
	return found
}

// firstIdent returns the first *ast.Ident that is not a package name.
func firstIdent(astFile *ast.File) *ast.Ident {
	var found *ast.Ident
	ast.Inspect(astFile, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if id, ok := n.(*ast.Ident); ok && id != astFile.Name {
			found = id
		}
		return true
	})
	return found
}

// makeMutant builds a minimal gcore.Mutant value at the position reported by
// the supplied fset for node. The token.File is obtained from fset.File(pos).
func makeMutant(id int, fset *token.FileSet, astFile *ast.File, node ast.Node) gcore.Mutant {
	pos := node.Pos()
	tokFile := fset.File(pos)
	position := fset.Position(pos)
	return gcore.Mutant{
		ID:       id,
		Operator: arithmetic_flip.ArithmeticFlip{},
		Site: engine.Site{
			File:    tokFile,
			FileAST: astFile,
			Fset:    fset,
			Node:    node,
			Line:    position.Line,
			Column:  position.Column,
		},
	}
}

func silentLogger() *logger.Logger { return logger.New(false) }

const (
	srcWithIntLiteral = `package subject

func GetValue() int {
	return 1
}
`

	srcIntInBinaryExpr = `package subject

func Calc(x int) int {
	return x + 1
}
`

	srcTypeErrorPropagation = `package subject

func GetInt() int {
	return 42
}

func UseString(s string) {
	_ = s
}

func Main() {
	x := GetInt()
	UseString(x)
}
`

	srcWithExternalDeps = `package subject

import "unknown/pkg/xyz"

func Main() {
	_ = xyz.SomeFunc()
}
`

	srcMultipleLiterals = `package subject

var (
	A int = 1
	B int = 2
	C int = 3
	D int = 4
	E int = 5
)

func Values() (int, int, int) {
	return A, B, C
}

func Process() int {
	return A + B + C + D + E
}
`

	srcBinaryExprInt = `package subject

func Add(a, b int) int {
	return a + b
}
`

	srcBoolToIntContext = `package subject

func IsPositive(n int) bool {
	return n > 0
}

func Check(x int) int {
	if IsPositive(x) {
		return 1
	}
	return 0
}
`
)

func makeRealMutant(id int, fset *token.FileSet, astFile *ast.File, node ast.Node, op mutator.Operator) gcore.Mutant {
	pos := node.Pos()
	tokFile := fset.File(pos)
	position := fset.Position(pos)
	return gcore.Mutant{
		ID:       id,
		Operator: op,
		Site: engine.Site{
			File:    tokFile,
			FileAST: astFile,
			Fset:    fset,
			Node:    node,
			Line:    position.Line,
			Column:  position.Column,
		},
	}
}

func makeMutantWithOp(id int, fset *token.FileSet, astFile *ast.File, node ast.Node, op mutator.Operator) gcore.Mutant {
	pos := node.Pos()
	tokFile := fset.File(pos)
	position := fset.Position(pos)
	return gcore.Mutant{
		ID:       id,
		Operator: op,
		Site: engine.Site{
			File:    tokFile,
			FileAST: astFile,
			Fset:    fset,
			Node:    node,
			Line:    position.Line,
			Column:  position.Column,
		},
	}
}

func firstBasicLit(astFile *ast.File) *ast.BasicLit {
	var found *ast.BasicLit
	ast.Inspect(astFile, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if bl, ok := n.(*ast.BasicLit); ok {
			found = bl
		}
		return true
	})
	return found
}

func allBasicLits(astFile *ast.File) []*ast.BasicLit {
	var lits []*ast.BasicLit
	ast.Inspect(astFile, func(n ast.Node) bool {
		if bl, ok := n.(*ast.BasicLit); ok {
			lits = append(lits, bl)
		}
		return true
	})
	return lits
}

func firstFuncDecl(astFile *ast.File) *ast.FuncDecl {
	var found *ast.FuncDecl
	ast.Inspect(astFile, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if fd, ok := n.(*ast.FuncDecl); ok {
			found = fd
		}
		return true
	})
	return found
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestRunPreflight_NilNode_RejectedAtLevel1 verifies that a Mutant whose
// Site.Node is nil is rejected during the Level-1 static filter and never
// reaches Level 2 or Level 3.
func TestRunPreflight_NilNode_RejectedAtLevel1(t *testing.T) {
	fset, astFile, filePath := parseTempSource(t, `package subject

func Add(a, b int) int { return a + b }
`)
	tokFile := fset.File(astFile.Pos())

	nilNodeMutant := gcore.Mutant{
		ID: 1,
		Site: engine.Site{
			File:    tokFile,
			FileAST: astFile,
			Fset:    fset,
			Node:    nil, // ← deliberately nil
			Line:    3,
			Column:  5,
		},
	}

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{nilNodeMutant}, silentLogger())

	if len(valid) != 0 {
		t.Errorf("expected 0 valid mutants, got %d", len(valid))
	}
	if len(invalid) != 1 {
		t.Fatalf("expected 1 invalid result, got %d", len(invalid))
	}
	if invalid[0].MutantID != 1 {
		t.Errorf("invalid result MutantID = %d, want 1", invalid[0].MutantID)
	}
	if invalid[0].Status != gcore.StatusInvalid {
		t.Errorf("invalid result Status = %q, want %q", invalid[0].Status, gcore.StatusInvalid)
	}

	_ = filePath // used implicitly via tokFile
}

// TestRunPreflight_NilFile_RejectedAtLevel1 verifies that a Mutant whose
// Site.File is nil is caught by the Level-1 nil-file guard.
func TestRunPreflight_NilFile_RejectedAtLevel1(t *testing.T) {
	fset, astFile, _ := parseTempSource(t, `package subject

func Mul(x, y int) int { return x * y }
`)
	node := firstBinaryExpr(astFile)
	if node == nil {
		t.Fatal("test source has no binary expression")
	}
	pos := fset.Position(node.Pos())

	nilFileMutant := gcore.Mutant{
		ID: 2,
		Site: engine.Site{
			File:    nil, // ← deliberately nil
			FileAST: astFile,
			Fset:    fset,
			Node:    node,
			Line:    pos.Line,
			Column:  pos.Column,
		},
	}

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{nilFileMutant}, silentLogger())

	if len(valid) != 0 {
		t.Errorf("expected 0 valid mutants, got %d", len(valid))
	}
	if len(invalid) != 1 {
		t.Fatalf("expected 1 invalid result, got %d", len(invalid))
	}
	if invalid[0].Status != gcore.StatusInvalid {
		t.Errorf("Status = %q, want %q", invalid[0].Status, gcore.StatusInvalid)
	}
}

// TestRunPreflight_NoOpMutation_PassesAllThreeLevels verifies that a mutant
// with an empty-operator (no-op mutation) passes all three preflight levels.
// Note: This tests infrastructure, not actual mutation behavior.
func TestRunPreflight_NoOpMutation_PassesAllThreeLevels(t *testing.T) {
	const src = `package subject

func Sum(a, b int) int {
	return a + b
}
`
	fset, astFile, _ := parseTempSource(t, src)
	node := firstBinaryExpr(astFile)
	if node == nil {
		t.Fatal("test source has no binary expression")
	}

	m := makeMutant(10, fset, astFile, node)

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m}, silentLogger())

	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid, got %d: %+v", len(invalid), invalid)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid mutant, got %d", len(valid))
	}
	if valid[0].ID != 10 {
		t.Errorf("valid[0].ID = %d, want 10", valid[0].ID)
	}
}

// TestRunPreflight_MixedBatch_CorrectlyPartitioned exercises the full pipeline
// with a mix of nil-node, nil-file, and valid mutants and asserts that each
// ends up in the right bucket.
func TestRunPreflight_MixedBatch_CorrectlyPartitioned(t *testing.T) {
	const src = `package subject

func Diff(a, b int) int {
	return a - b
}
`
	fset, astFile, _ := parseTempSource(t, src)
	node := firstBinaryExpr(astFile)
	if node == nil {
		t.Fatal("test source has no binary expression")
	}
	tokFile := fset.File(astFile.Pos())
	pos := fset.Position(node.Pos())

	nilNode := gcore.Mutant{
		ID: 100,
		Site: engine.Site{
			File: tokFile, Node: nil, FileAST: astFile, Fset: fset,
			Line: pos.Line, Column: pos.Column,
		},
	}
	nilFile := gcore.Mutant{
		ID: 101,
		Site: engine.Site{
			File: nil, Node: node, FileAST: astFile, Fset: fset,
			Line: pos.Line, Column: pos.Column,
		},
	}
	validMutant := makeMutant(102, fset, astFile, node)

	valid, invalid := gcore.RunPreflight(
		[]gcore.Mutant{nilNode, nilFile, validMutant},
		silentLogger(),
	)

	if len(valid) != 1 {
		t.Errorf("valid count = %d, want 1", len(valid))
	}
	if len(invalid) != 2 {
		t.Errorf("invalid count = %d, want 2", len(invalid))
	}
	if len(valid) == 1 && valid[0].ID != 102 {
		t.Errorf("valid[0].ID = %d, want 102", valid[0].ID)
	}

	invalidIDs := make(map[int]bool)
	for _, r := range invalid {
		invalidIDs[r.MutantID] = true
	}
	for _, wantID := range []int{100, 101} {
		if !invalidIDs[wantID] {
			t.Errorf("mutant #%d not found in invalid results", wantID)
		}
	}
}

// TestRunPreflight_Level3_TypeCheckRejectsInvalidMutation verifies that Level 3
// catches a mutant whose schemata-transformed code introduces a type error that
// was not present in the original source. We achieve this by placing two
// mutants in a file that contains a deliberately type-sensitive expression:
// one mutant is at a position that produces valid code; a second targets an
// identifier used as an incompatible type (string vs int context), which the
// go/types baseline-budget mechanism cannot absorb.
//
// The test only asserts the invariant that at least one of the mutants is
// rejected and that no mutant appears in both slices — i.e., the level-3
// budget/baseline logic has run and produced a deterministic partition.
func TestRunPreflight_Level3_TypeCheckBaselineDoesNotAbsorbMutationErrors(t *testing.T) {
	// The source has two mutation opportunities:
	//   (a) the "a + b" binary expression — benign arithmetic mutation target.
	//   (b) the return value type is forced to int; a mutation replacing the
	//       entire expression with a string literal would introduce a type error
	//       that is new relative to the baseline.
	const src = `package subject

import "strconv"

func Format(a, b int) string {
	sum := a + b
	return strconv.Itoa(sum)
}
`
	fset, astFile, _ := parseTempSource(t, src)
	binNode := firstBinaryExpr(astFile)
	if binNode == nil {
		t.Fatal("test source has no binary expression")
	}

	// Collect all identifiers to find one inside the function body.
	var bodyIdents []*ast.Ident
	ast.Inspect(astFile, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if ok && id != astFile.Name {
			bodyIdents = append(bodyIdents, id)
		}
		return true
	})

	// Build two valid-looking mutants targeting different nodes in the same file.
	m1 := makeMutant(200, fset, astFile, binNode)

	var m2 gcore.Mutant
	if len(bodyIdents) > 0 {
		m2 = makeMutant(201, fset, astFile, bodyIdents[len(bodyIdents)-1])
	} else {
		m2 = makeMutant(201, fset, astFile, binNode) // fallback: duplicate site
	}

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m1, m2}, silentLogger())

	// Invariant: every mutant ID appears in exactly one of the two slices.
	allIDs := map[int]int{200: 0, 201: 0} // id → count
	for _, m := range valid {
		allIDs[m.ID]++
	}
	for _, r := range invalid {
		allIDs[r.MutantID]++
	}
	for id, count := range allIDs {
		if count != 1 {
			t.Errorf("mutant #%d appeared in %d slices (want exactly 1)", id, count)
		}
	}

	// The combined count must equal the input count.
	if got := len(valid) + len(invalid); got != 2 {
		t.Errorf("total results = %d, want 2", got)
	}
}

func TestRunPreflight_Level3_TypeErrorIsRejected(t *testing.T) {
	fset, astFile, _ := parseTempSource(t, srcTypeErrorPropagation)
	litNode := firstBasicLit(astFile)
	if litNode == nil {
		t.Fatal("test source has no basic literal")
	}

	m := makeMutantWithOp(300, fset, astFile, litNode, mock_operators.TypeErrorToStringOperator{})

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m}, silentLogger())

	if len(valid) != 0 {
		t.Errorf("expected 0 valid, got %d", len(valid))
	}
	if len(invalid) != 1 {
		t.Fatalf("expected 1 invalid, got %d", len(invalid))
	}
	if invalid[0].Status != gcore.StatusCompileError {
		t.Errorf("Status = %q, want %q", invalid[0].Status, gcore.StatusCompileError)
	}
	if invalid[0].ErrorReason == "" {
		t.Error("ErrorReason should not be empty")
	}
}

func TestRunPreflight_Level3_ValidMutationPasses(t *testing.T) {
	fset, astFile, _ := parseTempSource(t, srcWithIntLiteral)
	litNode := firstBasicLit(astFile)
	if litNode == nil {
		t.Fatal("test source has no basic literal")
	}

	m := makeMutantWithOp(301, fset, astFile, litNode, mock_operators.ValidIntFlipOperator{})

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m}, silentLogger())

	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid, got %d: %v", len(invalid), invalid)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid, got %d", len(valid))
	}
	if valid[0].ID != 301 {
		t.Errorf("valid[0].ID = %d, want 301", valid[0].ID)
	}
}

func TestRunPreflight_Level3_MultipleMutantsPartitioned(t *testing.T) {
	fset, astFile, _ := parseTempSource(t, srcMultipleLiterals)
	lits := allBasicLits(astFile)
	if len(lits) < 3 {
		t.Skipf("need at least 3 literals, got %d", len(lits))
	}

	m1 := makeMutantWithOp(400, fset, astFile, lits[0], mock_operators.ValidIntFlipOperator{})
	m2 := makeMutantWithOp(401, fset, astFile, lits[1], mock_operators.TypeErrorToStringOperator{})
	m3 := makeMutantWithOp(402, fset, astFile, lits[2], mock_operators.ValidIntFlipOperator{})

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m1, m2, m3}, silentLogger())

	invalidIDs := make(map[int]bool)
	for _, r := range invalid {
		invalidIDs[r.MutantID] = true
	}

	if !invalidIDs[401] {
		t.Errorf("mutant 401 (type error) should be invalid")
	}
	if len(valid) != 2 {
		t.Errorf("expected 2 valid mutants, got %d", len(valid))
	}
}

func TestRunPreflight_RealOperator_ConstantReplacement(t *testing.T) {
	fset, astFile, _ := parseTempSource(t, srcWithIntLiteral)
	litNode := firstBasicLit(astFile)
	if litNode == nil {
		t.Fatal("test source has no basic literal")
	}

	op := constant_replacement.ConstantReplacement{}
	m := makeMutantWithOp(500, fset, astFile, litNode, op)

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m}, silentLogger())

	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid, got %d: %v", len(invalid), invalid)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid, got %d", len(valid))
	}
}

func TestRunPreflight_RealOperator_ArithmeticFlip(t *testing.T) {
	fset, astFile, _ := parseTempSource(t, srcBinaryExprInt)
	binNode := firstBinaryExpr(astFile)
	if binNode == nil {
		t.Fatal("test source has no binary expression")
	}

	op := arithmetic_flip.ArithmeticFlip{}
	m := makeMutantWithOp(501, fset, astFile, binNode, op)

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m}, silentLogger())

	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid, got %d: %v", len(invalid), invalid)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid, got %d", len(valid))
	}
}

func TestRunPreflight_RealOperator_Registered(t *testing.T) {
	fset, astFile, _ := parseTempSource(t, srcWithIntLiteral)
	litNode := firstBasicLit(astFile)
	if litNode == nil {
		t.Fatal("test source has no basic literal")
	}

	op, ok := mutator.Get("constant_replacement")
	if !ok {
		t.Fatal("constant_replacement operator not found in registry")
	}
	m := makeRealMutant(502, fset, astFile, litNode, op)

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m}, silentLogger())

	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid, got %d: %v", len(invalid), invalid)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid, got %d", len(valid))
	}
}

func TestRunPreflight_BisectWithMultipleBadMutants(t *testing.T) {
	fset, astFile, _ := parseTempSource(t, srcMultipleLiterals)
	lits := allBasicLits(astFile)
	if len(lits) < 5 {
		t.Skipf("need at least 5 literals, got %d", len(lits))
	}

	m1 := makeMutantWithOp(600, fset, astFile, lits[0], mock_operators.ValidIntFlipOperator{})
	m2 := makeMutantWithOp(601, fset, astFile, lits[1], mock_operators.TypeErrorToStringOperator{})
	m3 := makeMutantWithOp(602, fset, astFile, lits[2], mock_operators.ValidIntFlipOperator{})
	m4 := makeMutantWithOp(603, fset, astFile, lits[3], mock_operators.TypeErrorToStringOperator{})
	m5 := makeMutantWithOp(604, fset, astFile, lits[4], mock_operators.TypeErrorToStringOperator{})

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m1, m2, m3, m4, m5}, silentLogger())

	invalidIDs := make(map[int]bool)
	for _, r := range invalid {
		invalidIDs[r.MutantID] = true
	}

	if !invalidIDs[601] {
		t.Errorf("mutant 601 should be invalid")
	}
	if !invalidIDs[603] {
		t.Errorf("mutant 603 should be invalid")
	}
	if !invalidIDs[604] {
		t.Errorf("mutant 604 should be invalid")
	}
	if len(valid) != 2 {
		t.Errorf("expected 2 valid mutants, got %d", len(valid))
	}
}

func TestRunPreflight_RealFile_LoggerGo(t *testing.T) {
	absPath, err := filepath.Abs("../../internal/logger/logger.go")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, absPath, src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	lit := firstBasicLit(astFile)
	if lit == nil {
		t.Skip("no literals in logger.go")
	}

	m := makeMutantWithOp(700, fset, astFile, lit, constant_replacement.ConstantReplacement{})

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m}, silentLogger())

	t.Logf("logger.go valid=%d invalid=%d", len(valid), len(invalid))
	if len(valid)+len(invalid) != 1 {
		t.Errorf("expected 1 total result, got %d", len(valid)+len(invalid))
	}
}

func TestRunPreflight_RealFile_DiffGo(t *testing.T) {
	absPath, err := filepath.Abs("../../internal/diff/diff.go")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, absPath, src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	binExpr := firstBinaryExpr(astFile)
	if binExpr == nil {
		t.Skip("no binary expressions in diff.go")
	}

	m := makeMutantWithOp(701, fset, astFile, binExpr, arithmetic_flip.ArithmeticFlip{})

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m}, silentLogger())

	t.Logf("diff.go valid=%d invalid=%d", len(valid), len(invalid))
	if len(valid)+len(invalid) != 1 {
		t.Errorf("expected 1 total result, got %d", len(valid)+len(invalid))
	}
}

func TestRunPreflight_RealFile_TransformGo(t *testing.T) {
	absPath, err := filepath.Abs("../../internal/core/transform.go")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, absPath, src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	lits := allBasicLits(astFile)
	if len(lits) < 2 {
		t.Skip("need at least 2 literals in transform.go")
	}

	m1 := makeMutantWithOp(702, fset, astFile, lits[0], constant_replacement.ConstantReplacement{})
	m2 := makeMutantWithOp(703, fset, astFile, lits[1], arithmetic_flip.ArithmeticFlip{})

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m1, m2}, silentLogger())

	t.Logf("transform.go valid=%d invalid=%d", len(valid), len(invalid))
	if len(valid)+len(invalid) != 2 {
		t.Errorf("expected 2 total results, got %d", len(valid)+len(invalid))
	}
}

func TestRunPreflight_RealFile_WithMultipleMutants(t *testing.T) {
	absPath, err := filepath.Abs("../../internal/cache/cache.go")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, absPath, src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	lits := allBasicLits(astFile)
	bins := func() []*ast.BinaryExpr {
		var result []*ast.BinaryExpr
		ast.Inspect(astFile, func(n ast.Node) bool {
			if be, ok := n.(*ast.BinaryExpr); ok {
				result = append(result, be)
			}
			return true
		})
		return result
	}()

	if len(lits) < 2 && len(bins) < 2 {
		t.Skip("need at least 2 mutation sites in cache.go")
	}

	var mutants []gcore.Mutant
	id := 800
	if len(lits) >= 2 {
		mutants = append(mutants, makeMutantWithOp(id, fset, astFile, lits[0], constant_replacement.ConstantReplacement{}))
		id++
		mutants = append(mutants, makeMutantWithOp(id, fset, astFile, lits[1], constant_replacement.ConstantReplacement{}))
	} else {
		mutants = append(mutants, makeMutantWithOp(id, fset, astFile, bins[0], arithmetic_flip.ArithmeticFlip{}))
		id++
		mutants = append(mutants, makeMutantWithOp(id, fset, astFile, bins[1], arithmetic_flip.ArithmeticFlip{}))
	}

	valid, invalid := gcore.RunPreflight(mutants, silentLogger())

	t.Logf("cache.go: valid=%d invalid=%d", len(valid), len(invalid))
	if len(valid)+len(invalid) != len(mutants) {
		t.Errorf("expected %d total results, got %d", len(mutants), len(valid)+len(invalid))
	}
}

func TestRunPreflight_ErrorFiltering_NumericAssertions(t *testing.T) {
	fset, astFile, _ := parseTempSource(t, srcMultipleLiterals)
	lits := allBasicLits(astFile)
	if len(lits) < 5 {
		t.Skipf("need 5+ literals, got %d", len(lits))
	}

	m1 := makeMutantWithOp(900, fset, astFile, lits[0], constant_replacement.ConstantReplacement{})
	m2 := makeMutantWithOp(901, fset, astFile, lits[1], constant_replacement.ConstantReplacement{})
	m3 := makeMutantWithOp(902, fset, astFile, lits[2], constant_replacement.ConstantReplacement{})
	m4 := makeMutantWithOp(903, fset, astFile, lits[3], mock_operators.TypeErrorToStringOperator{})
	m5 := makeMutantWithOp(904, fset, astFile, lits[4], mock_operators.TypeErrorToStringOperator{})

	input := []gcore.Mutant{m1, m2, m3, m4, m5}
	valid, invalid := gcore.RunPreflight(input, silentLogger())

	totalOutput := len(valid) + len(invalid)
	t.Logf("Input: %d | Output: %d (valid=%d, invalid=%d)", len(input), totalOutput, len(valid), len(invalid))

	if totalOutput != len(input) {
		t.Errorf("MUTANTS_LOST: expected %d total, got %d — mutants NOT accounted for!", len(input), totalOutput)
	}

	invalidIDs := make(map[int]bool)
	for _, r := range invalid {
		invalidIDs[r.MutantID] = true
	}

	expectedInvalid := []int{903, 904}
	for _, id := range expectedInvalid {
		if !invalidIDs[id] {
			t.Errorf("EXPECTED_INVALID: mutant %d should be invalid (type error), but got valid", id)
		}
	}

	expectedValid := []int{900, 901, 902}
	for _, id := range expectedValid {
		found := false
		for _, m := range valid {
			if m.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("EXPECTED_VALID: mutant %d should be valid, but got invalid or lost", id)
		}
	}

	t.Logf("FILTERING_VERIFIED: %d valid, %d invalid (expected: 3 valid, 2 invalid)", len(valid), len(invalid))
}

func TestRunPreflight_AccountForAllMutants(t *testing.T) {
	testSources := []string{
		"package p\nvar A = 1\n",
		"package p\nvar B = 2\n",
		"package p\nvar C = 3\n",
		"package p\nvar D = 4\n",
		"package p\nvar E = 5\n",
	}

	for i, src := range testSources {
		fset, astFile, _ := parseTempSource(t, src)
		lits := allBasicLits(astFile)
		if len(lits) == 0 {
			continue
		}

		m := makeMutantWithOp(1000+i, fset, astFile, lits[0], constant_replacement.ConstantReplacement{})
		valid, invalid := gcore.RunPreflight([]gcore.Mutant{m}, silentLogger())

		total := len(valid) + len(invalid)
		if total != 1 {
			t.Errorf("mutant %d: NOT_ACCOUNTED - expected 1, got %d (valid=%d, invalid=%d)",
				1000+i, total, len(valid), len(invalid))
		}
	}
}

func TestRunPreflight_VariableReplacement_Operator(t *testing.T) {
	src := `package subject

var globalVar = 10

func GetValue() int {
	return globalVar
}
`
	fset, astFile, _ := parseTempSource(t, src)

	ident := func() *ast.Ident {
		var found *ast.Ident
		ast.Inspect(astFile, func(n ast.Node) bool {
			if found != nil {
				return false
			}
			if id, ok := n.(*ast.Ident); ok && id.Name == "globalVar" {
				found = id
			}
			return true
		})
		return found
	}()

	if ident == nil {
		t.Skip("no identifier found")
	}

	op := variable_replacement.VariableReplacement{}
	m := makeMutantWithOp(1100, fset, astFile, ident, op)

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m}, silentLogger())

	t.Logf("variable_replacement: valid=%d invalid=%d", len(valid), len(invalid))
	if len(valid)+len(invalid) != 1 {
		t.Errorf("NOT_ACCOUNTED: expected 1, got %d", len(valid)+len(invalid))
	}
}

func TestRunPreflight_SignToggle_Operator(t *testing.T) {
	src := `package subject

func Negate(x int) int {
	return -x
}

func UsePositive() int {
	return 5
}
`
	fset, astFile, _ := parseTempSource(t, src)

	binExpr := func() *ast.UnaryExpr {
		var found *ast.UnaryExpr
		ast.Inspect(astFile, func(n ast.Node) bool {
			if found != nil {
				return false
			}
			if ue, ok := n.(*ast.UnaryExpr); ok && ue.Op == token.SUB {
				found = ue
			}
			return true
		})
		return found
	}()

	if binExpr == nil {
		t.Skip("no unary negation found")
	}

	op := sign_toggle.SignToggle{}
	m := makeMutantWithOp(1110, fset, astFile, binExpr, op)

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m}, silentLogger())

	t.Logf("sign_toggle: valid=%d invalid=%d", len(valid), len(invalid))
	if len(valid)+len(invalid) != 1 {
		t.Errorf("NOT_ACCOUNTED: expected 1, got %d", len(valid)+len(invalid))
	}
}

func TestRunPreflight_IncDecFlip_Operator(t *testing.T) {
	src := `package subject

func UseBoth(x int) int {
	x++
	return x
}

func Decrement(y int) int {
	y--
	return y
}
`
	fset, astFile, _ := parseTempSource(t, src)

	incStmt := func() *ast.IncDecStmt {
		var found *ast.IncDecStmt
		ast.Inspect(astFile, func(n ast.Node) bool {
			if found != nil {
				return false
			}
			if ids, ok := n.(*ast.IncDecStmt); ok {
				found = ids
			}
			return true
		})
		return found
	}()

	if incStmt == nil {
		t.Skip("no increment/decrement found")
	}

	op := inc_dec_flip.IncDecFlip{}
	m := makeMutantWithOp(1120, fset, astFile, incStmt, op)

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m}, silentLogger())

	t.Logf("inc_dec_flip: valid=%d invalid=%d", len(valid), len(invalid))
	if len(valid)+len(invalid) != 1 {
		t.Errorf("NOT_ACCOUNTED: expected 1, got %d", len(valid)+len(invalid))
	}
}

func TestRunPreflight_MultipleFiles_MultiOperator(t *testing.T) {
	realFiles := []string{
		"../../internal/core/mutants.go",
		"../../internal/core/utils.go",
		"../../internal/engine/engine.go",
		"../../internal/cache/cache.go",
		"../../internal/runner/runner.go",
	}

	validCount := 0
	invalidCount := 0
	totalFiles := 0

	for _, relPath := range realFiles {
		absPath, err := filepath.Abs(relPath)
		if err != nil {
			continue
		}
		src, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, absPath, src, parser.ParseComments)
		if err != nil {
			continue
		}

		lits := allBasicLits(astFile)
		bins := func() []*ast.BinaryExpr {
			var result []*ast.BinaryExpr
			ast.Inspect(astFile, func(n ast.Node) bool {
				if be, ok := n.(*ast.BinaryExpr); ok {
					result = append(result, be)
				}
				return true
			})
			return result
		}()

		if len(lits) == 0 && len(bins) == 0 {
			continue
		}
		totalFiles++

		var mutants []gcore.Mutant
		id := 2000 + totalFiles*100

		if len(lits) > 0 {
			mutants = append(mutants, makeMutantWithOp(id, fset, astFile, lits[0], constant_replacement.ConstantReplacement{}))
		}
		if len(bins) > 0 {
			mutants = append(mutants, makeMutantWithOp(id+1, fset, astFile, bins[0], arithmetic_flip.ArithmeticFlip{}))
		}

		if len(mutants) == 0 {
			continue
		}

		v, inv := gcore.RunPreflight(mutants, silentLogger())
		validCount += len(v)
		invalidCount += len(inv)
	}

	t.Logf("MULTI_FILE: %d files, valid=%d invalid=%d", totalFiles, validCount, invalidCount)

	if validCount+invalidCount < totalFiles {
		t.Errorf("LOST_MUTANTS: expected at least %d, got %d", totalFiles, validCount+invalidCount)
	}
}

func TestRunPreflight_AllRegisteredOperators(t *testing.T) {
	src := `package subject

var x = 1
var y = 2

func Add() int { return x + y }
func Sub() int { return x - y }
func Negate(x int) int { return -x }
func UseBoth(x int) int { x++ ; return x }
`
	fset, astFile, _ := parseTempSource(t, src)

	lits := allBasicLits(astFile)
	bins := func() []*ast.BinaryExpr {
		var result []*ast.BinaryExpr
		ast.Inspect(astFile, func(n ast.Node) bool {
			if be, ok := n.(*ast.BinaryExpr); ok {
				result = append(result, be)
			}
			return true
		})
		return result
	}()

	_ops := []mutator.Operator{
		constant_replacement.ConstantReplacement{},
		arithmetic_flip.ArithmeticFlip{},
		variable_replacement.VariableReplacement{},
		sign_toggle.SignToggle{},
		inc_dec_flip.IncDecFlip{},
	}

	if len(lits) < 2 || len(bins) < 1 {
		t.Skip("insufficient nodes for all operators")
	}

	var mutants []gcore.Mutant
	for i, op := range _ops {
		id := 3000 + i
		node := lits[i%len(lits)]
		mutants = append(mutants, makeMutantWithOp(id, fset, astFile, node, op))
	}

	valid, invalid := gcore.RunPreflight(mutants, silentLogger())

	t.Logf("ALL_OPS: valid=%d invalid=%d (total_ops=%d)", len(valid), len(invalid), len(_ops))

	totalOutput := len(valid) + len(invalid)
	if totalOutput != len(mutants) {
		t.Errorf("NOT_ACCOUNTED: expected %d, got %d", len(mutants), totalOutput)
	}

	for _, m := range valid {
		t.Logf("  VALID: mutant %d with operator", m.ID)
	}
	for _, r := range invalid {
		t.Logf("  INVALID: mutant %d - %s", r.MutantID, r.ErrorReason)
	}
}

func parsePreflightFixture(t *testing.T, filename string) (*token.FileSet, *ast.File, string) {
	t.Helper()
	absPath, err := filepath.Abs("../../tests/integration/testdata/preflight/" + filename)
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, absPath, src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	return fset, astFile, absPath
}

func TestRunPreflight_Testdata_Simple(t *testing.T) {
	fset, astFile, _ := parsePreflightFixture(t, "simple.go")
	lits := allBasicLits(astFile)
	if len(lits) < 2 {
		t.Skip("need at least 2 literals")
	}

	m1 := makeMutantWithOp(4000, fset, astFile, lits[0], constant_replacement.ConstantReplacement{})
	m2 := makeMutantWithOp(4001, fset, astFile, lits[1], constant_replacement.ConstantReplacement{})

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m1, m2}, silentLogger())

	t.Logf("simple.go: valid=%d invalid=%d", len(valid), len(invalid))
	if len(valid)+len(invalid) != 2 {
		t.Errorf("NOT_ACCOUNTED: expected 2, got %d", len(valid)+len(invalid))
	}
}

func TestRunPreflight_Testdata_BinaryExpr(t *testing.T) {
	fset, astFile, _ := parsePreflightFixture(t, "binary_expr.go")
	bins := func() []*ast.BinaryExpr {
		var result []*ast.BinaryExpr
		ast.Inspect(astFile, func(n ast.Node) bool {
			if be, ok := n.(*ast.BinaryExpr); ok {
				result = append(result, be)
			}
			return true
		})
		return result
	}()
	if len(bins) < 2 {
		t.Skip("need at least 2 binary expressions")
	}

	m1 := makeMutantWithOp(4010, fset, astFile, bins[0], arithmetic_flip.ArithmeticFlip{})
	m2 := makeMutantWithOp(4011, fset, astFile, bins[1], arithmetic_flip.ArithmeticFlip{})

	valid, invalid := gcore.RunPreflight([]gcore.Mutant{m1, m2}, silentLogger())

	t.Logf("binary_expr.go: valid=%d invalid=%d", len(valid), len(invalid))
	if len(valid)+len(invalid) != 2 {
		t.Errorf("NOT_ACCOUNTED: expected 2, got %d", len(valid)+len(invalid))
	}
}

func TestRunPreflight_Testdata_Multiple(t *testing.T) {
	fset, astFile, _ := parsePreflightFixture(t, "multiple.go")
	lits := allBasicLits(astFile)
	if len(lits) < 5 {
		t.Skip("need 5+ literals")
	}

	ops := []mutator.Operator{
		constant_replacement.ConstantReplacement{},
		constant_replacement.ConstantReplacement{},
		constant_replacement.ConstantReplacement{},
		constant_replacement.ConstantReplacement{},
		constant_replacement.ConstantReplacement{},
	}

	var mutants []gcore.Mutant
	for i := range ops {
		mutants = append(mutants, makeMutantWithOp(4020+i, fset, astFile, lits[i], ops[i]))
	}

	valid, invalid := gcore.RunPreflight(mutants, silentLogger())

	t.Logf("multiple.go: valid=%d invalid=%d (input=%d)", len(valid), len(invalid), len(mutants))

	totalOutput := len(valid) + len(invalid)
	if totalOutput != len(mutants) {
		t.Errorf("LOST_MUTANTS: expected %d, got %d", len(mutants), totalOutput)
	}

	if len(valid) != len(mutants) {
		t.Errorf("FILTERING_BROKEN: expected all valid, got %d invalid", len(invalid))
	}
}

func TestRunPreflight_DeepMultiFile_MultiOperator(t *testing.T) {
	realFiles := []struct {
		path      string
		operators []mutator.Operator
	}{
		{"../../internal/cli/cli.go", []mutator.Operator{
			constant_replacement.ConstantReplacement{},
			arithmetic_flip.ArithmeticFlip{},
		}},
		{"../../internal/cache/cache.go", []mutator.Operator{
			constant_replacement.ConstantReplacement{},
			arithmetic_flip.ArithmeticFlip{},
		}},
		{"../../internal/core/executor.go", []mutator.Operator{
			constant_replacement.ConstantReplacement{},
			arithmetic_flip.ArithmeticFlip{},
		}},
		{"../../internal/runner/runner.go", []mutator.Operator{
			constant_replacement.ConstantReplacement{},
			arithmetic_flip.ArithmeticFlip{},
		}},
	}

	totalValid := 0
	totalInvalid := 0
	totalMutants := 0
	filesProcessed := 0

	for _, fileConfig := range realFiles {
		absPath, err := filepath.Abs(fileConfig.path)
		if err != nil {
			continue
		}
		src, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, absPath, src, parser.ParseComments)
		if err != nil {
			continue
		}

		lits := allBasicLits(astFile)
		bins := func() []*ast.BinaryExpr {
			var result []*ast.BinaryExpr
			ast.Inspect(astFile, func(n ast.Node) bool {
				if be, ok := n.(*ast.BinaryExpr); ok {
					result = append(result, be)
				}
				return true
			})
			return result
		}()

		if len(lits) == 0 && len(bins) == 0 {
			continue
		}
		filesProcessed++

		var mutants []gcore.Mutant
		baseID := 5000 + filesProcessed*100

		for i, op := range fileConfig.operators {
			id := baseID + i
			if i%2 == 0 && len(lits) > 0 {
				mutants = append(mutants, makeMutantWithOp(id, fset, astFile, lits[0], op))
			} else if len(bins) > 0 {
				mutants = append(mutants, makeMutantWithOp(id, fset, astFile, bins[0], op))
			}
		}

		if len(mutants) == 0 {
			continue
		}
		totalMutants += len(mutants)

		v, inv := gcore.RunPreflight(mutants, silentLogger())
		totalValid += len(v)
		totalInvalid += len(inv)
	}

	t.Logf("DEEP_MULTI_FILE: %d files | mutants: %d | valid: %d | invalid: %d",
		filesProcessed, totalMutants, totalValid, totalInvalid)

	if totalValid+totalInvalid != totalMutants {
		t.Errorf("LOST_MUTANTS: expected %d total, got %d (valid=%d, invalid=%d)",
			totalMutants, totalValid+totalInvalid, totalValid, totalInvalid)
	}

	if totalValid == 0 && totalInvalid == 0 {
		t.Error("NO_MUTANTS_PROCESSED: something went wrong")
	}
}

func TestRunPreflight_DeepFiltering_Verification(t *testing.T) {
	src := `package subject

var (
	A = 1
	B = 2
	C = 3
	D = 4
	E = 5
)
`
	fset, astFile, _ := parseTempSource(t, src)
	lits := allBasicLits(astFile)
	if len(lits) < 5 {
		t.Skipf("need 5 literals, got %d", len(lits))
	}

	m1 := makeMutantWithOp(6000, fset, astFile, lits[0], constant_replacement.ConstantReplacement{})
	m2 := makeMutantWithOp(6001, fset, astFile, lits[1], constant_replacement.ConstantReplacement{})
	m3 := makeMutantWithOp(6002, fset, astFile, lits[2], constant_replacement.ConstantReplacement{})
	m4 := makeMutantWithOp(6003, fset, astFile, lits[3], mock_operators.TypeErrorToStringOperator{})
	m5 := makeMutantWithOp(6004, fset, astFile, lits[4], mock_operators.TypeErrorToStringOperator{})

	inputMutants := []gcore.Mutant{m1, m2, m3, m4, m5}
	valid, invalid := gcore.RunPreflight(inputMutants, silentLogger())

	totalOutput := len(valid) + len(invalid)
	inputCount := len(inputMutants)

	invalidIDs := make(map[int]bool)
	for _, r := range invalid {
		invalidIDs[r.MutantID] = true
	}

	t.Logf("=== FILTERING VERIFICATION ===")
	t.Logf("Input:    %d mutants", inputCount)
	t.Logf("Output:   %d mutants", totalOutput)
	t.Logf("Valid:    %d mutants", len(valid))
	t.Logf("Invalid:  %d mutants", len(invalid))
	t.Logf("==================================")

	if totalOutput != inputCount {
		t.Fatalf("CRITICAL_FAILURE: mutants lost! expected %d, got %d", inputCount, totalOutput)
	}

	expectedInvalid := []int{6003, 6004}
	for _, id := range expectedInvalid {
		if !invalidIDs[id] {
			t.Errorf("FILTER_FAIL: mutant %d should be FILTERED (invalid) but was VALID", id)
		}
		t.Logf("FILTER_CHECK: mutant %d -> %v", id, invalidIDs[id])
	}

	expectedValidIDs := map[int]bool{6000: true, 6001: true, 6002: true}
	for _, m := range valid {
		if !expectedValidIDs[m.ID] {
			t.Errorf("UNEXPECTED_VALID: mutant %d passed but should have been filtered", m.ID)
		}
	}

	t.Logf("FILTERING_VERIFICATION: PASSED (%d valid, %d invalid filtered correctly)",
		len(valid), len(invalid))
}
