# Makefile – Gorgon

.PHONY: all verify fmt update-lint lint lint-fix vet test coverage build clean help

all: verify build

verify: fmt vet lint test coverage
	@echo "Verification passed ✓"

fmt:
	@echo "→ Formatting code..."
	go fmt ./...

vet:
	@echo "→ Running go vet..."
	go vet ./...

update-lint:
	rm -f ./bin/golangci-lint*
	$(MAKE) lint
	./bin/golangci-lint --version

lint:
	@echo "→ Running golangci-lint (latest)..."
	@if [ ! -f ./bin/golangci-lint ]; then \
		echo "→ Installing latest golangci-lint into ./bin/..."; \
		curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b ./bin || { echo "Install failed"; exit 1; }; \
	fi
	./bin/golangci-lint run --timeout=5m

lint-fix:
	@echo "→ Running golangci-lint with --fix..."
	@if [ ! -f ./bin/golangci-lint ]; then $(MAKE) lint; fi
	./bin/golangci-lint run --timeout=5m --fix

test:
	@echo "→ Running tests (with race detector)..."
	go test ./... -race -count=1

coverage:
	@echo "→ Generating coverage report..."
	go test ./... -race -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

build:
	@echo "→ Building binary..."
	go build -o bin/gorgon ./cmd/gorgon

clean:
	rm -f coverage.out coverage.html bin/gorgon ./bin/golangci-lint*

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'