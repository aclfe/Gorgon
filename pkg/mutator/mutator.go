// Package mutator provides mutation operators for the gorgon project
package mutator

import (
	"go/ast"
	"go/token"
)

type Context struct {
	ReturnType    string
	FunctionName  string
	PackageName   string
	FileName      string
	Position      token.Position
	EnclosingFunc *ast.FuncDecl
	File          *ast.File
	Parent        ast.Node
}

type Operator interface {
	Name() string
	CanApply(node ast.Node) bool
	Mutate(node ast.Node) ast.Node
}

type ContextualOperator interface {
	Operator
	CanApplyWithContext(node ast.Node, ctx Context) bool
	MutateWithContext(node ast.Node, ctx Context) ast.Node
}

// ApplyOperator applies an operator to a node, handling both regular and contextual operators.
// For contextual operators, it creates a context with the provided parameters and calls MutateWithContext.
// For regular operators, it calls Mutate directly.
// Returns the mutated node, or nil if the operator cannot be applied.
func ApplyOperator(op Operator, node ast.Node, returnType string, file *ast.File, enclosingFunc *ast.FuncDecl) ast.Node {
	ctx := Context{
		ReturnType:    returnType,
		File:          file,
		EnclosingFunc: enclosingFunc,
	}
	if cop, ok := op.(ContextualOperator); ok {
		return cop.MutateWithContext(node, ctx)
	}
	return op.Mutate(node)
}

type OperatorInitializer func() Operator

type OperatorRegistry struct {
	operators    map[string]Operator
	initializers map[string]OperatorInitializer
}

func NewOperatorRegistry() *OperatorRegistry {
	return &OperatorRegistry{
		operators:    make(map[string]Operator),
		initializers: make(map[string]OperatorInitializer),
	}
}

func (r *OperatorRegistry) Register(op Operator) {
	r.operators[op.Name()] = op
}

func (r *OperatorRegistry) RegisterInitializer(name string, init OperatorInitializer) {
	r.initializers[name] = init
}

func (r *OperatorRegistry) Get(name string) (Operator, bool) {
	if op, ok := r.operators[name]; ok {
		return op, true
	}
	if init, ok := r.initializers[name]; ok {
		op := init()
		r.operators[name] = op
		return op, true
	}
	return nil, false
}

func (r *OperatorRegistry) List() []Operator {
	ops := make([]Operator, 0, len(r.operators))
	for _, op := range r.operators {
		ops = append(ops, op)
	}
	for name, init := range r.initializers {
		if _, exists := r.operators[name]; !exists {
			ops = append(ops, init())
		}
	}
	return ops
}

func (r *OperatorRegistry) All() map[string]Operator {
	result := make(map[string]Operator)
	for k, v := range r.operators {
		result[k] = v
	}
	for name, init := range r.initializers {
		if _, exists := result[name]; !exists {
			result[name] = init()
		}
	}
	return result
}

var globalRegistry = NewOperatorRegistry()

var categoryMap = map[string][]string{
	"arithmetic": {
		"arithmetic_flip",
	},
	"logical": {
		"condition_negation",
		"negate_condition",
		"logical_operator",
	},
	"boundary": {
		"boundary_value",
	},
	"assignment": {
		"assignment_operator",
	},
	"function_body": {
		"empty_body",
	},
	"reference_returns": {
		"pointer_returns",
		"slice_returns",
		"map_returns",
		"interface_returns",
		"channel_returns",
	},
	"switch_mutations": {
		"switch_remove_default",
		"swap_case_bodies",
	},
	"zero_value_return": {
		"zero_value_return_numeric",
		"zero_value_return_string",
		"zero_value_return_bool",
		"zero_value_return_error",
	},
	"binary": {
		"binary_math",
		"inc_dec_flip",
		"sign_toggle",
	},
	"literal": {
		"constant_replacement",
		"variable_replacement",
	},
	"early_return": {
		"early_return_removal",
	},
	"loop": {
		"loop_body_removal",
		"loop_break_first",
		"loop_break_removal",
	},
	"statement": {
		"defer_removal",
	},
	"conditional_expression": {
		"if_condition_true",
		"if_condition_false",
		"for_condition_true",
		"for_condition_false",
	},
}

func Register(op Operator) {
	globalRegistry.Register(op)
}

func Get(name string) (Operator, bool) {
	return globalRegistry.Get(name)
}

func MustGet(name string) Operator {
	op, ok := globalRegistry.Get(name)
	if !ok {
		panic("operator not found: " + name)
	}
	return op
}

func List() []Operator {
	return globalRegistry.List()
}

func All() map[string]Operator {
	return globalRegistry.All()
}

func GetCategory(name string) ([]Operator, bool) {
	names, ok := categoryMap[name]
	if !ok {
		return nil, false
	}
	result := make([]Operator, 0, len(names))
	for _, n := range names {
		if op, ok := globalRegistry.Get(n); ok {
			result = append(result, op)
		}
	}
	return result, true
}

func ListCategories() []string {
	cats := make([]string, 0, len(categoryMap))
	for k := range categoryMap {
		cats = append(cats, k)
	}
	return cats
}
