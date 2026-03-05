VERSION ?= $(shell grep "const Version =" pkg/version/version.go | cut -d'"' -f2)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")

.PHONY: build-all clean help

# Default target
help:
	@echo "OpenExec Build System v$(VERSION)"
	@echo "  make build-all    Build binaries for all platforms using Docker"
	@echo "  make clean        Remove build artifacts"

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

clean:
	rm -rf dist/
	rm -rf ui/dist
	docker rmi openexec-builder || true
