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


.PHONY: all check tidy test coverage deadcode vet lint clean help build install cross-compile

all: check build install cross-compile
	$(SECTION)
	@echo "$(GREEN)$(BOLD)All checks passed. PR away.$(RESET)"
	$(SECTION)

check: tidy vet test coverage deadcode lint
	$(SUCCESS) Full local validation complete.

tidy:
	$(SECTION)
	$(INFO) Tidying Go modules...
	@go mod tidy
	@go mod download 2>&1 | grep -v "no module dependencies" || true
	$(SUCCESS) Go modules tidied.

test:
	$(SECTION)
	$(INFO) Running unit tests with race detector...
	@go test -v -race ./...
	$(SUCCESS) Unit tests passed.

coverage:
	$(SECTION)
	$(INFO) Generating coverage report...
	@go test -coverprofile=coverage.out ./...
	@echo ""
	@octocov
	$(SUCCESS) Coverage metrics generated.

deadcode:
	$(SECTION)
	$(INFO) Checking for unreachable code...
	@if ! command -v deadcode >/dev/null 2>&1; then \
		$(WARN) deadcode missing - installing...; \
		go install golang.org/x/tools/cmd/deadcode@latest; \
	fi
	@deadcode -test ./...
	$(SUCCESS) No unreachable code found.

lint:
	$(SECTION)
	$(INFO) Running golangci-lint...
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		$(ERROR) golangci-lint missing.; \
		echo "$(YELLOW)Install with:$(RESET) ./scripts/setup.sh"; \
		exit 1; \
	fi
	@golangci-lint run --timeout=5m --color=always ./...
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
	@echo "$(DIM)One-time setup: ./scripts/setup.sh$(RESET)"