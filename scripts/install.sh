#!/bin/bash
set -e

# OpenExec Installation Script
# Builds the unified Go binary containing CLI, Orchestration, and UI.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

echo "=== OpenExec Unified Installation ==="
echo ""

# Check prerequisites
check_prereqs() {
    if ! command -v go &> /dev/null; then
        echo "❌ Error: 'go' (1.24+) is required to build OpenExec."
        exit 1
    fi
}

# Build Go Binary
build_go() {
    echo "📦 Building unified OpenExec binary..."
    cd "$ROOT_DIR"

    mkdir -p bin
    
    # Build with optimization flags
    go build -trimpath -ldflags="-s -w" -o bin/openexec ./cmd/openexec
    
    echo "✅ Built: $ROOT_DIR/bin/openexec"
}

# Main
check_prereqs
build_go

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Add to your PATH:"
echo "  export PATH=\"$ROOT_DIR/bin:\$PATH\""
echo ""
echo "Quick start:"
echo "  cd your-project"
echo "  openexec init"
echo "  openexec wizard"
echo "  openexec plan INTENT.md"
echo "  openexec run"
