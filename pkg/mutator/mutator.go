// Package mutator provides mutation operators for the gorgon project
package mutator

import (
	"go/ast"
	"go/token"
)

type Context struct {
	ReturnType     string
	FunctionName   string
	PackageName    string
	FileName       string
	Position       token.Position
	EnclosingFunc  *ast.FuncDecl
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

func Register(op Operator) {
	globalRegistry.Register(op)
}

func Get(name string) (Operator, bool) {
	return globalRegistry.Get(name)
}

func List() []Operator {
	return globalRegistry.List()
}

func All() map[string]Operator {
	return globalRegistry.All()
}
