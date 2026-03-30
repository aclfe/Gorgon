package switch_mutations

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type SwapCaseBodies struct{}

func (SwapCaseBodies) Name() string {
	return "swap_case_bodies"
}

func (SwapCaseBodies) CanApply(n ast.Node) bool {
	cc, ok := n.(*ast.CaseClause)
	if !ok {
		return false
	}
	if cc.List == nil {
		return false
	}
	return len(cc.Body) > 0
}

func (SwapCaseBodies) Mutate(n ast.Node) ast.Node {
	return nil
}

func (SwapCaseBodies) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	return SwapCaseBodies{}.CanApply(n)
}

func (SwapCaseBodies) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	cc, ok := n.(*ast.CaseClause)
	if !ok || cc.List == nil || len(cc.Body) == 0 {
		return nil
	}

	if ctx.File == nil {
		return nil
	}

	siblings := findSiblingCasesInSameSwitch(cc, ctx.File)
	if len(siblings) < 2 {
		return nil
	}

	currentIndex := -1
	for i, c := range siblings {
		if c == cc {
			currentIndex = i
			break
		}
	}

	if currentIndex < 0 {
		return nil
	}

	var swapIndex int
	if currentIndex == len(siblings)-1 {
		swapIndex = currentIndex - 1
	} else {
		swapIndex = currentIndex + 1
	}

	swapCase := siblings[swapIndex]
	if len(swapCase.Body) == 0 {
		return nil
	}

	return &ast.CaseClause{
		Case:  cc.Case,
		List:  cc.List,
		Colon: cc.Colon,
		Body:  swapCase.Body,
	}
}

func findCasesInFile(cc *ast.CaseClause, file *ast.File) []*ast.CaseClause {
	var siblings []*ast.CaseClause

	ast.Inspect(file, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.SwitchStmt:
			for _, s := range stmt.Body.List {
				if c, ok := s.(*ast.CaseClause); ok {
					siblings = append(siblings, c)
				}
			}
			return false
		case *ast.TypeSwitchStmt:
			for _, s := range stmt.Body.List {
				if c, ok := s.(*ast.CaseClause); ok {
					siblings = append(siblings, c)
				}
			}
			return false
		}
		return true
	})

	return siblings
}

func findSiblingCasesInSameSwitch(cc *ast.CaseClause, file *ast.File) []*ast.CaseClause {
	var siblings []*ast.CaseClause
	var targetSwitch ast.Node

	ast.Inspect(file, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.SwitchStmt:
			for _, s := range stmt.Body.List {
				if c, ok := s.(*ast.CaseClause); ok {
					if c == cc {
						targetSwitch = stmt
						return false
					}
				}
			}
		case *ast.TypeSwitchStmt:
			for _, s := range stmt.Body.List {
				if c, ok := s.(*ast.CaseClause); ok {
					if c == cc {
						targetSwitch = stmt
						return false
					}
				}
			}
		}
		return true
	})

	if targetSwitch == nil {
		return nil
	}

	switch stmt := targetSwitch.(type) {
	case *ast.SwitchStmt:
		for _, s := range stmt.Body.List {
			if c, ok := s.(*ast.CaseClause); ok {
				siblings = append(siblings, c)
			}
		}
	case *ast.TypeSwitchStmt:
		for _, s := range stmt.Body.List {
			if c, ok := s.(*ast.CaseClause); ok {
				siblings = append(siblings, c)
			}
		}
	}

	return siblings
}

func init() {
	mutator.Register(SwapCaseBodies{})
}
