.PHONY: build-all clean help

# Default target
help:
	@echo "OpenExec Build System"
	@echo "  make build-all    Build binaries for all platforms using Docker"
	@echo "  make clean        Remove build artifacts"

build-all:
	@echo "🚀 Building multi-platform binaries via Docker..."
	docker build -f Dockerfile.build -t openexec-builder .
	@echo "📦 Extracting binaries to ./dist..."
	mkdir -p dist
	docker run --rm -v $(PWD)/dist:/output openexec-builder sh -c "cp /* /output/"
	@echo "✅ Binaries ready in ./dist:"
	@ls -lh dist/

clean:
	rm -rf dist/
	rm -rf ui/dist
	docker rmi openexec-builder || true
