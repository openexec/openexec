#!/bin/bash
set -e

# OpenExec Monorepo Installation Script
# Builds all Go binaries and installs Orchestration Engine (Python)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
ORCH_DIR="$ROOT_DIR/../openexec-planner"

echo "=== OpenExec Installation (Monorepo) ==="
echo ""

# Check prerequisites
check_prereqs() {
    local missing=()

    if ! command -v go &> /dev/null; then
        missing+=("go (1.24+)")
    fi

    if ! command -v pip &> /dev/null && ! command -v pip3 &> /dev/null; then
        missing+=("pip/pip3 (Python 3.11+)")
    fi

    if [ ${#missing[@]} -ne 0 ]; then
        echo "Missing prerequisites:"
        for dep in "${missing[@]}"; do
            echo "  - $dep"
        done
        exit 1
    fi
}

# Build Go Binaries
build_go() {
    echo "[1/2] Building Go Binaries..."
    cd "$ROOT_DIR"

    mkdir -p bin
    
    echo "      Building openexec (CLI)..."
    go build -trimpath -ldflags="-s -w" -o bin/openexec ./cmd/openexec
    
    echo "      Building openexec-engine (Daemon)..."
    go build -trimpath -ldflags="-s -w" -o bin/openexec-engine ./cmd/openexec-engine
    
    echo "      Building openexec-interface (Gateway)..."
    go build -trimpath -ldflags="-s -w" -o bin/openexec-interface ./cmd/openexec-interface
    
    # Note: axon is now a subcommand of openexec, but we keep the binary for backwards compat if needed
    echo "      Building axon (Legacy)..."
    go build -trimpath -ldflags="-s -w" -o bin/axon ./cmd/axon

    echo "      Built: $ROOT_DIR/bin/"
}

# Install orchestration
install_orchestration() {
    echo "[2/2] Installing Orchestration Engine..."

    if [ ! -d "$ORCH_DIR" ]; then
        echo "      Warning: $ORCH_DIR not found"
        echo "      Skipping orchestration engine install..."
        return 0
    fi

    cd "$ORCH_DIR"
    pip install -e . --quiet

    echo "      Installed: openexec-planner"
}

# Main
check_prereqs
build_go
install_orchestration

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Add to your PATH:"
echo "  export PATH="$ROOT_DIR/bin:\$PATH""
echo ""
echo "Or add to ~/.bashrc or ~/.zshrc:"
echo "  echo 'export PATH="$ROOT_DIR/bin:\$PATH"' >> ~/.zshrc"
echo ""
echo "Verify installation:"
echo "  openexec --help"
echo "  openexec-engine --help"
echo ""
echo "Quick start:"
echo "  cd your-project"
echo "  openexec init"
echo "  openexec wizard"
echo "  openexec plan INTENT.md"
echo "  openexec story import"
echo "  openexec start --daemon"
echo "  openexec run"
