// Package analysis provides AST traversal, position checking, and node cloning
// utilities used by mutation operators.
package analysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"sync"
)

func IsReturnStmtWithResults(n ast.Node, min int) (*ast.ReturnStmt, bool) {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) < min {
		return nil, false
	}
	return ret, true
}

var (
	caseClauseCache = make(map[string]bool)
	caseClauseMu    sync.RWMutex
)

func caseClauseCacheKey(ret *ast.ReturnStmt, file *ast.File) string {
	return fmt.Sprintf("%p:%d", file, ret.Pos())
}

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
	retPos := ret.Pos()
	var result bool
	ast.Inspect(file, func(n ast.Node) bool {
		if n == ret {
			return true
		}
		if cc, ok := n.(*ast.CaseClause); ok {
			if retPos >= cc.Pos() && retPos < cc.End() {
				result = true
				return false
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

func FindNode(node, target ast.Node) bool {
	return findNode(node, target)
}

func FindParentNode(target ast.Node, file *ast.File, predicates ...func(ast.Node) bool) ast.Node {
	if file == nil {
		return nil
	}

	targetPos := target.Pos()
	targetEnd := target.End()
	if targetPos == token.NoPos {
		return findParentByTraversal(target, file, predicates...)
	}

	var closest ast.Node
	var closestPos token.Pos

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil || n == target {
			return true
		}
		if n.Pos() > targetPos || n.End() < targetEnd {
			return true
		}
		for _, pred := range predicates {
			if pred(n) {
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

func findParentByTraversal(target ast.Node, file *ast.File, predicates ...func(ast.Node) bool) ast.Node {
	var result ast.Node
	var resultPos token.Pos

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil || n == target {
			return true
		}
		for _, pred := range predicates {
			if pred(n) && containsNode(n, target) {
				if result == nil || n.Pos() > resultPos {
					result = n
					resultPos = n.Pos()
				}
				break
			}
		}
		return true
	})

	return result
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

func IsInsideIfStmt(n ast.Node, parent ast.Node) bool {
	_, ok := parent.(*ast.IfStmt)
	return ok
}

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
