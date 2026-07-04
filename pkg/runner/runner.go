// Package runner is the PUBLIC seam for resolving a model name to the agent
// runner command that launches it. Out-of-tree modules (e.g. a governance
// product layer) depend on this instead of reaching into internal/runner, so
// the resolution logic stays MIT and self-contained while the public surface
// remains small and stable.
package runner

import internalrunner "github.com/openexec/openexec/internal/runner"

// Resolve maps a model name to the executable and arguments that launch the
// corresponding agent runner. overrideCmd/overrideArgs, when non-empty, take
// precedence over the model-derived defaults.
func Resolve(model, overrideCmd string, overrideArgs []string) (cmd string, args []string, err error) {
	return internalrunner.Resolve(model, overrideCmd, overrideArgs)
}
