package mcp

import "strings"

// shellIdempotentAllowlist lists commands that are considered idempotent for
// the purpose of skipping re-execution when an identical idempotency key was
// observed. This list is intentionally small and conservative.
var shellIdempotentAllowlist = []string{
    "git",      // e.g., git status, git diff (read-only)
    "go",       // e.g., go version, go env (read-only)
    "node",     // e.g., --version
    "npm",      // e.g., npm --version
    "yarn",     // e.g., yarn --version
    "python3",  // e.g., --version
    "python",   // e.g., --version
}

// IsShellIdempotent returns true if the provided command appears in the
// allowlist and arguments look read-only (best-effort heuristic).
// Callers should still prefer treating shell as non-idempotent unless
// explicitly safe.
func IsShellIdempotent(command string, args []string) bool {
    c := strings.ToLower(strings.TrimSpace(command))
    if c == "" { return false }
    allowed := false
    for _, a := range shellIdempotentAllowlist {
        if c == a { allowed = true; break }
    }
    if !allowed { return false }

    // Basic arg heuristics: deny if args include likely mutating verbs
    denySubs := []string{"install", "remove", "delete", "apply", "commit", "push", "pull", "write"}
    for _, a := range args {
        la := strings.ToLower(a)
        for _, d := range denySubs {
            if strings.Contains(la, d) {
                return false
            }
        }
    }
    return true
}

