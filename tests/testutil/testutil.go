package testutil

import (
	"github.com/aclfe/gorgon/internal/logger"
)

var testLogger = logger.New(false)

func Logger() *logger.Logger {
	return testLogger
}
