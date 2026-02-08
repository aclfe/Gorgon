#!/usr/bin/env bash

set -euo pipefail

echo "=== Installing/updating required tools ==="

rm -f $(which golangci-lint)
rm -f $(which deadcode)
go clean -modcache
rm -rf ~/.cache/go-build

echo "1. golangci-lint v2"
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

echo "2. deadcode"
go install golang.org/x/tools/cmd/deadcode@master

echo "3. octocov"
go install github.com/k1LoW/octocov@latest

echo ""
echo "Done! Verify versions:"
golangci-lint --version
go version
deadcode --version 2>/dev/null || echo "deadcode installed (no --version flag)"
octocov --version 2>/dev/null || echo "octocov installed (no --version flag)"
