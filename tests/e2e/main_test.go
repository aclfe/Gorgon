//go:build e2e
// +build e2e

package e2e

import (
	"flag"
	"os"
	"testing"
	"time"
)

// TestMain extends the default `go test -timeout` (10m) to 30m for the e2e
// package. The four TestExternalSuites_* tests target internal/core (~4k
// mutants each) and run sequentially; their combined runtime exceeds the
// default alarm and produces a "test timed out after 10m0s" panic. We bump
// the flag before m.Run() so the testing harness arms its alarm with the
// new value. A user-supplied -timeout (anything >= 30m) is left alone.
func TestMain(m *testing.M) {
	const minTimeout = 30 * time.Minute
	// Force flag parsing now — without this, the timeout flag still holds its
	// registered default (0s) and `go test`'s injected -timeout=10m0s isn't
	// applied until m.Run() calls flag.Parse() internally, which is too late
	// to override.
	if !flag.Parsed() {
		flag.Parse()
	}
	if f := flag.Lookup("test.timeout"); f != nil {
		if cur, err := time.ParseDuration(f.Value.String()); err == nil {
			if cur > 0 && cur < minTimeout {
				_ = f.Value.Set(minTimeout.String())
			}
		}
	}
	os.Exit(m.Run())
}
