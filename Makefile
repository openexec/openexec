VERSION ?= $(shell grep "const Version =" pkg/version/version.go | cut -d'"' -f2)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")

.PHONY: all build build-all clean lint test type-check ui-lint ui-test ui-type-check help

# Default target
help:
	@echo "OpenExec Build System v$(VERSION)"
	@echo "  make build        Build the openexec binary for current platform"
	@echo "  make build-all    Build binaries for all platforms using Docker"
	@echo "  make lint         Run all linters (Go + UI)"
	@echo "  make test         Run all unit tests (Go + UI)"
	@echo "  make type-check   Run all type checking (Go + UI)"
	@echo "  make ui-lint      Run ESLint for the UI"
	@echo "  make ui-test      Run Vitest for the UI"
	@echo "  make ui-type-check Run tsc for the UI"
	@echo "  make clean        Remove build artifacts"

build:
	@echo "🚀 Building OpenExec binary..."
	go build -o bin/openexec ./cmd/openexec

build-all:
	@echo "🚀 Building multi-platform binaries v$(VERSION) ($(COMMIT)) via Docker..."
	docker build -f Dockerfile.build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		-t openexec-builder .
	@echo "📦 Extracting binaries to ./dist..."
	mkdir -p dist
	docker run --rm -v $(PWD)/dist:/output openexec-builder sh -c "cp /* /output/"
	@echo "✅ Binaries ready in ./dist:"
	@ls -lh dist/

lint:
	@echo "🔍 Linting Go (vet)..."
	go vet ./...
	@if command -v golangci-lint >/dev/null; then \
		echo "🔍 Linting Go (golangci-lint)..."; \
		golangci-lint run; \
	fi
	@$(MAKE) ui-lint

ui-lint:
	@echo "🔍 Linting UI..."
	cd ui && npm run lint

test:
	@echo "🧪 Running Go tests..."
	go test ./...
	@$(MAKE) ui-test

ui-test:
	@echo "🧪 Running UI tests..."
	cd ui && npm run test

type-check:
	@echo "⌨️ Checking Go types..."
	go build -v ./...
	@$(MAKE) ui-type-check

ui-type-check:
	@echo "⌨️ Checking UI types..."
	cd ui && npm run type-check

clean:
	rm -rf dist/
	rm -rf bin/
	rm -rf ui/dist
	docker rmi openexec-builder || true
