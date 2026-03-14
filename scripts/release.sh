#!/bin/bash
set -e

if [ -z "$1" ]; then
  echo "Usage: ./scripts/release.sh <version> (e.g., 0.3.4)"
  exit 1
fi

VERSION=$1
echo "🚀 Releasing v$VERSION..."

# Go to the root of the openexec repository
cd "$(dirname "$0")/.."
OPENEXEC_DIR=$(pwd)
# Assume siblings are openexec-web and openexec-docs
PROJECTS_DIR=$(dirname "$OPENEXEC_DIR")

echo "1. Bumping versions..."

# 1. openexec/pkg/version/version.go
sed -i '' -e "s/const Version = \".*\"/const Version = \"$VERSION\"/" "$OPENEXEC_DIR/pkg/version/version.go"

# 2. openexec-web/package.json
sed -i '' -e "1,10s/\"version\": \".*\"/\"version\": \"$VERSION\"/" "$PROJECTS_DIR/openexec-web/package.json"

# 3. openexec-web/public/install.sh
sed -i '' -e "s/VERSION=\".*\"/VERSION=\"$VERSION\"/" "$PROJECTS_DIR/openexec-web/public/install.sh"

# 4. openexec-web/src/pages/Index.tsx
sed -i '' -e "s/openexec — v[0-9]*\.[0-9]*\.[0-9]*/openexec — v$VERSION/g" "$PROJECTS_DIR/openexec-web/src/pages/Index.tsx"
sed -i '' -e "s/DIRECT DOWNLOADS (v[0-9]*\.[0-9]*\.[0-9]*)/DIRECT DOWNLOADS (v$VERSION)/g" "$PROJECTS_DIR/openexec-web/src/pages/Index.tsx"

# 5. openexec-web/src/components/TerminalLog.tsx
sed -i '' -e "s/Release v[0-9]*\.[0-9]*\.[0-9]*/Release v$VERSION/" "$PROJECTS_DIR/openexec-web/src/components/TerminalLog.tsx"

# 6. openexec-docs/docusaurus.config.ts
sed -i '' -e "s/Operating System v[0-9]*\.[0-9]*\.[0-9]*/Operating System v$VERSION/" "$PROJECTS_DIR/openexec-docs/docusaurus.config.ts"

# 7. openexec-docs/docs/why-openexec.md
sed -i '' -e "s/OpenExec v[0-9]*\.[0-9]*\.[0-9]*/OpenExec v$VERSION/g" "$PROJECTS_DIR/openexec-docs/docs/why-openexec.md"

echo "2. Building OpenExec CLI..."
cd "$OPENEXEC_DIR/ui"
npm run build
cd "$OPENEXEC_DIR"

COMMIT=$(git rev-parse --short HEAD)
# -s: Omit the symbol table and debug information.
# -w: Omit the DWARF symbol table.
LDFLAGS="-s -w -X github.com/openexec/openexec/pkg/version.Commit=$COMMIT"

mkdir -p bin

echo "   - Building local binary..."
go build -ldflags "$LDFLAGS" -o openexec ./cmd/openexec

echo "   - Building darwin/arm64..."
GOOS=darwin GOARCH=arm64 go build -ldflags "$LDFLAGS" -o bin/openexec-darwin-arm64 ./cmd/openexec

echo "   - Building darwin/amd64..."
GOOS=darwin GOARCH=amd64 go build -ldflags "$LDFLAGS" -o bin/openexec-darwin-amd64 ./cmd/openexec

echo "   - Building linux/amd64..."
GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o bin/openexec-linux-amd64 ./cmd/openexec

echo "   - Building linux/arm64..."
GOOS=linux GOARCH=arm64 go build -ldflags "$LDFLAGS" -o bin/openexec-linux-arm64 ./cmd/openexec

echo "   - Building windows/amd64..."
GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS" -o bin/openexec-windows-amd64.exe ./cmd/openexec

echo "3. Copying binaries to openexec-web..."
cp "$OPENEXEC_DIR/bin/openexec-darwin-arm64" "$PROJECTS_DIR/openexec-web/public/downloads/"
cp "$OPENEXEC_DIR/bin/openexec-darwin-amd64" "$PROJECTS_DIR/openexec-web/public/downloads/"
cp "$OPENEXEC_DIR/bin/openexec-linux-amd64" "$PROJECTS_DIR/openexec-web/public/downloads/"
cp "$OPENEXEC_DIR/bin/openexec-linux-arm64" "$PROJECTS_DIR/openexec-web/public/downloads/"
cp "$OPENEXEC_DIR/bin/openexec-windows-amd64.exe" "$PROJECTS_DIR/openexec-web/public/downloads/"

echo "$VERSION" > "$PROJECTS_DIR/openexec-web/public/version.txt"

echo "4. Building Web & Docs..."
cd "$PROJECTS_DIR/openexec-web"
npm run build

cd "$PROJECTS_DIR/openexec-docs"
npm run build

echo "5. Committing and Pushing..."
# openexec-web
cd "$PROJECTS_DIR/openexec-web"
git add .
git commit -m "release: v$VERSION (optimized binary sizes)" || true
# git push origin main || true

# openexec-docs
cd "$PROJECTS_DIR/openexec-docs"
git add .
git commit -m "docs: bump version to v$VERSION" || true
# git push origin main || true

# openexec
cd "$OPENEXEC_DIR"
git add .
git commit -m "release: v$VERSION (optimized binary sizes)" || true
# git push origin main || true
git tag -f "v$VERSION"
# git push origin -f "v$VERSION"

echo "✅ Release v$VERSION completed successfully!"
