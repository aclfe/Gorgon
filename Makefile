# Makefile for Gorgon

RESET   := \033[0m
BOLD    := \033[1m
DIM     := \033[2m
RED     := \033[31m
GREEN   := \033[32m
YELLOW  := \033[33m
BLUE    := \033[34m
MAGENTA := \033[35m
CYAN    := \033[36m

INFO    := @echo "$(BLUE)[INFO]$(RESET)"
SUCCESS := @echo "$(GREEN)[✓]$(RESET)"
ERROR   := @echo "$(RED)[✗]$(RESET)"
WARN    := @echo "$(YELLOW)[WARN]$(RESET)"
SECTION := @echo "\n$(BOLD)$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
BINARY_NAME := gorgon
BUILD_DIR   := bin

PKGS := $(shell go list ./... | grep -v "cmd/gorgon$$" | grep -v "examples")
COVER_PKGS := $(shell echo $(PKGS) | sed 's/ /,/g')

.PHONY: all check tidy test coverage deadcode vet lint clean help build install cross-compile bench bench-compare bench-mem

all: check build install cross-compile
	$(SECTION)
	@echo "$(GREEN)$(BOLD)All checks passed. PR away.$(RESET)"
	$(SECTION)

check: tidy vet vulncheck fmt test coverage deadcode lint
	$(SUCCESS) Full local validation complete.

tidy:
	$(SECTION)
	$(INFO) Tidying Go modules...
	@go mod tidy
	@go mod download 2>&1 | grep -v "no module dependencies" || true
	$(SUCCESS) Go modules tidied.

fmt:
	$(SECTION)
	$(INFO) Formatting code with gofumpt...
	@go run mvdan.cc/gofumpt@latest -w .
	$(SUCCESS) Code formatted.

test:
	$(SECTION)
	@go clean -testcache
	$(INFO) Running unit tests with race detector...
	@go test -v -race $(PKGS)
	$(SUCCESS) Unit tests passed.

coverage:
	$(SECTION)
	$(INFO) Generating coverage report...
	@go clean -testcache
	@go test -coverpkg=$(COVER_PKGS) -coverprofile=coverage.out $(PKGS)
	@go tool cover -html=coverage.out -o coverage.html
	@echo ""
	@octocov
	$(SUCCESS) Coverage metrics generated. Open coverage.html to view details.

deadcode:
	$(SECTION)
	$(INFO) Checking for unreachable code...
	@if ! command -v deadcode >/dev/null 2>&1; then \
		$(WARN) deadcode missing - installing...; \
		go install golang.org/x/tools/cmd/deadcode@latest; \
	fi
	@deadcode -test ./...
	$(SUCCESS) No unreachable code found.

lint-fix:
	$(SECTION)
	$(INFO) Running golangci-lint with auto-fix...
	@golangci-lint run --config .golangci.yml --timeout=5m --color=always --fix ./...
	$(SUCCESS) Auto-fixes applied.

lint:
	$(SECTION)
	$(INFO) Running golangci-lint...
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		$(ERROR) golangci-lint missing.; \
		echo "$(YELLOW)Install with:$(RESET) ./scripts/setup.sh"; \
		exit 1; \
	fi
	@golangci-lint cache clean
	@golangci-lint run --config .golangci.yml --timeout=5m --color=always ./...
	$(SUCCESS) Lint passed.

vet:
	$(SECTION)
	$(INFO) Running go vet...
	@go vet ./...
	@if command -v staticcheck >/dev/null 2>&1; then \
		echo "$(BLUE)[INFO]$(RESET) Running staticcheck..."; \
		staticcheck ./...; \
	fi
	$(SUCCESS) Vet passed.

vulncheck:
	$(SECTION)
	$(INFO) Running govulncheck...
	@if ! command -v govulncheck >/dev/null 2>&1; then \
		$(WARN) govulncheck missing - installing...; \
		go install golang.org/x/vuln/cmd/govulncheck; \
	fi
	@govulncheck ./...
	$(SUCCESS) No vulnerabilities found.

bench:
	$(SECTION)
	$(INFO) Running benchmarks...
	@go test -bench=. -benchmem -benchtime=3s ./internal/engine
	$(SUCCESS) Benchmarks complete.

bench-compare:
	$(SECTION)
	$(INFO) Running comparative benchmarks...
	@echo "$(CYAN)Running benchmarks and saving to bench-old.txt...$(RESET)"
	@go test -bench=. -benchmem -benchtime=3s ./internal/engine > bench-old.txt
	@echo ""
	@echo "$(YELLOW)Make your code changes, then run:$(RESET)"
	@echo "  $(GREEN)go test -bench=. -benchmem -benchtime=3s ./internal/engine > bench-new.txt$(RESET)"
	@echo "  $(GREEN)benchstat bench-old.txt bench-new.txt$(RESET)"
	@echo ""
	@echo "$(DIM)Install benchstat with: go install golang.org/x/perf/cmd/benchstat@latest$(RESET)"
	$(SUCCESS) Baseline saved to bench-old.txt

bench-mem:
	$(SECTION)
	$(INFO) Running memory-focused benchmarks...
	@go test -bench=. -benchmem -memprofile=mem.out -cpuprofile=cpu.out ./internal/engine
	@echo ""
	@echo "$(CYAN)View memory profile:$(RESET) go tool pprof -http=:8080 mem.out"
	@echo "$(CYAN)View CPU profile:$(RESET)    go tool pprof -http=:8080 cpu.out"
	$(SUCCESS) Profiles saved to mem.out and cpu.out

build:
	$(SECTION)
	$(INFO) Building $(BINARY_NAME)...
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/gorgon
	$(SUCCESS) Built → $(BUILD_DIR)/$(BINARY_NAME)

install:
	$(SECTION)
	$(INFO) Installing $(BINARY_NAME) to GOPATH...
	@go install ./cmd/gorgon
	$(SUCCESS) Installed to $$(go env GOPATH)/bin/$(BINARY_NAME)

cross-compile:
	$(SECTION)
	$(INFO) Building for multiple platforms...
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/gorgon
	@GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/gorgon
	@GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/gorgon
	@GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/gorgon
	$(SUCCESS) Cross-compiled all platforms to $(BUILD_DIR)/

clean:
	$(SECTION)
	$(INFO) Cleaning generated files...
	@rm -f coverage.out coverage.html
	@rm -f bench-old.txt bench-new.txt mem.out cpu.out
	@rm -rf $(BUILD_DIR)
	@go clean
	$(SUCCESS) Cleaned.

help:
	@echo "$(BOLD)$(CYAN)Gorgon Makefile Commands$(RESET)"
	@echo ""
	@echo "$(BOLD)Main:$(RESET)"
	@echo "  $(GREEN)make check$(RESET)      → Full local CI simulation $(DIM)(recommended before PR)$(RESET)"
	@echo "  $(GREEN)make all$(RESET)        → Alias for check"
	@echo ""
	@echo "$(BOLD)Individual Tasks:$(RESET)"
	@echo "  $(CYAN)make tidy$(RESET)       → go mod tidy + download"
	@echo "  $(CYAN)make test$(RESET)       → Run tests with race detector"
	@echo "  $(CYAN)make coverage$(RESET)   → Generate coverage report with octocov"
	@echo "  $(CYAN)make deadcode$(RESET)   → Check for unreachable functions"
	@echo "  $(CYAN)make lint$(RESET)       → Run golangci-lint"
	@echo "  $(CYAN)make vet$(RESET)        → Run go vet + staticcheck"
	@echo "  $(CYAN)make clean$(RESET)      → Remove generated files"
	@echo ""
	@echo "$(BOLD)Benchmarking:$(RESET)"
	@echo "  $(MAGENTA)make bench$(RESET)         → Run performance benchmarks"
	@echo "  $(MAGENTA)make bench-compare$(RESET) → Save baseline for before/after comparison"
	@echo "  $(MAGENTA)make bench-mem$(RESET)     → Generate memory & CPU profiles"
	@echo ""
	@echo "$(DIM)One-time setup: ./scripts/setup.sh$(RESET)"