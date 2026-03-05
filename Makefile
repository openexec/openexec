VERSION ?= $(shell grep "const Version =" pkg/version/version.go | cut -d'"' -f2)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")

.PHONY: all build build-all clean lint test type-check help

# Default target
help:
	@echo "OpenExec Build System v$(VERSION)"
	@echo "  make build        Build the openexec binary for current platform"
	@echo "  make build-all    Build binaries for all platforms using Docker"
	@echo "  make lint         Run linters for Go and UI"
	@echo "  make test         Run all unit tests"
	@echo "  make type-check   Run TypeScript type checking"
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
	@echo "🔍 Linting Go..."
	go vet ./...
	@echo "🔍 Linting UI..."
	cd ui && npm run lint

test:
	@echo "🧪 Running Go tests..."
	go test ./...
	@echo "🧪 Running UI tests..."
	cd ui && npm run test

type-check:
	@echo "⌨️ Checking UI types..."
	cd ui && npm run type-check

clean:
	rm -rf dist/
	rm -rf bin/
	rm -rf ui/dist
	docker rmi openexec-builder || true
