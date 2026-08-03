package version

// Version is the release version. A var, not a const, so release builds
// stamp it from the git tag via -ldflags; this default only appears in
// untagged local builds and must not be trusted as provenance.
var Version = "0.13.1"

// Commit is the git commit hash, usually injected at build time
var Commit = "none"
