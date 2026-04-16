package testing

import (
	"time"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/pkg/mutator"
)

type Mutant struct {
	ID           int
	Site         engine.Site
	Operator     mutator.Operator
	TempDir      string
	TempLine     int
	TempCol      int
	Status       string
	Error        error
	KilledBy     string
	KillDuration time.Duration
	KillOutput   string
	ErrorReason  string
}
