// Package common provides shared utilities for mutation operators.
package common

import (
	"fmt"
	"go/ast"
	"go/token"
	"sync"
)

// IsReturnStmtWithResults checks if n is a *ast.ReturnStmt with at least min results.
// Returns the typed return statement and true if the check passes.
func IsReturnStmtWithResults(n ast.Node, min int) (*ast.ReturnStmt, bool) {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) < min {
		return nil, false
	}
	return ret, true
}

// caseClauseCache caches isInsideCaseClause results per (filePtr:returnPos).
// Since the AST is parsed once per run, pointer addresses are stable within a process.
var (
	caseClauseCache = make(map[string]bool)
	caseClauseMu    sync.RWMutex
)

func caseClauseCacheKey(ret *ast.ReturnStmt, file *ast.File) string {
	return fmt.Sprintf("%p:%d", file, ret.Pos())
}

// IsInsideCaseClause checks if a return statement is inside a case clause.
// Uses caching for performance across multiple calls.
func IsInsideCaseClause(ret *ast.ReturnStmt, file *ast.File) bool {
	if file == nil {
		return false
	}

	key := caseClauseCacheKey(ret, file)

	caseClauseMu.RLock()
	cached, ok := caseClauseCache[key]
	caseClauseMu.RUnlock()
	if ok {
		return cached
	}

	result := isInsideCaseClauseSlow(ret, file)

	caseClauseMu.Lock()
	caseClauseCache[key] = result
	caseClauseMu.Unlock()

	return result
}

func isInsideCaseClauseSlow(ret *ast.ReturnStmt, file *ast.File) bool {
	var result bool
	ast.Inspect(file, func(n ast.Node) bool {
		if n == ret {
			return true
		}
		if cc, ok := n.(*ast.CaseClause); ok {
			for _, stmt := range cc.Body {
				if findNode(stmt, ret) {
					result = true
					return false
				}
			}
		}
		return true
	})
	return result
}

func findNode(node, target ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if n == target {
			found = true
			return false
		}
		return true
	})
	return found
}

// FindNode checks if target exists in the AST subtree rooted at node.
func FindNode(node, target ast.Node) bool {
	return findNode(node, target)
}

// FindParentNode finds the nearest ancestor of target in file's AST that matches
// any of the provided predicate functions. Returns nil if no match is found.
func FindParentNode(target ast.Node, file *ast.File, predicates ...func(ast.Node) bool) ast.Node {
	if file == nil {
		return nil
	}

	// First pass: confirm target exists and get its position
	var targetPos token.Pos
	ast.Inspect(file, func(n ast.Node) bool {
		if n == target {
			targetPos = n.Pos()
			return false
		}
		return true
	})
	if targetPos == 0 {
		return nil
	}

	// Second pass: find the nearest container matching any predicate
	// Track the closest match by position
	var closest ast.Node
	var closestPos token.Pos

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil || n.Pos() > targetPos || n.End() < targetPos {
			return true
		}
		for _, pred := range predicates {
			if pred(n) && containsNode(n, target) {
				if closest == nil || n.Pos() > closestPos {
					closest = n
					closestPos = n.Pos()
				}
				break
			}
		}
		return true
	})

	return closest
}

func containsNode(container, target ast.Node) bool {
	found := false
	ast.Inspect(container, func(n ast.Node) bool {
		if n == target {
			found = true
			return false
		}
		return true
	})
	return found
}

// IsInsideLoop checks if a node is inside a for or range loop.
func IsInsideLoop(n ast.Node, file *ast.File) bool {
	return FindParentNode(n, file, isLoopNode) != nil
}

func isLoopNode(n ast.Node) bool {
	switch n.(type) {
	case *ast.ForStmt, *ast.RangeStmt:
		return true
	}
	return false
}

// IsInsideIfStmt checks if a node is inside an if statement.
func IsInsideIfStmt(n ast.Node, parent ast.Node) bool {
	_, ok := parent.(*ast.IfStmt)
	return ok
}

// CloneBlockStmt creates a shallow copy of a BlockStmt to avoid AST sharing issues.
func CloneBlockStmt(stmt *ast.BlockStmt) *ast.BlockStmt {
	if stmt == nil {
		return nil
	}
	newList := make([]ast.Stmt, len(stmt.List))
	copy(newList, stmt.List)
	return &ast.BlockStmt{
		Lbrace: stmt.Lbrace,
		List:   newList,
		Rbrace: stmt.Rbrace,
	}
}

// CloneStmt creates a shallow copy of a statement. For if-else chains, this prevents
// the mutated AST from sharing references with the original AST.
func CloneStmt(stmt ast.Stmt) ast.Stmt {
	if stmt == nil {
		return nil
	}
	if ifStmt, ok := stmt.(*ast.IfStmt); ok {
		return &ast.IfStmt{
			If:   ifStmt.If,
			Init: ifStmt.Init,
			Cond: ifStmt.Cond,
			Body: CloneBlockStmt(ifStmt.Body),
			Else: CloneStmt(ifStmt.Else),
		}
	}
	if block, ok := stmt.(*ast.BlockStmt); ok {
		return CloneBlockStmt(block)
	}
	return stmt
}

// SwapBinaryToken looks up the token swap mapping in pairs (O(1) map lookup).
// Returns the new token and true if a swap exists, or the original token and false otherwise.
func SwapBinaryToken(op token.Token, pairs map[token.Token]token.Token) (token.Token, bool) {
	newOp, ok := pairs[op]
	return newOp, ok
}
