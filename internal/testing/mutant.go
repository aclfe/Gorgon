package testing

import (
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/pkg/mutator"
)

type Mutant struct {
	ID         int
	Site       engine.Site
	Operator   mutator.Operator
	PackageDir string
	TempDir    string
	Status     string
	Error      error
}
