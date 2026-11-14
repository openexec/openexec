package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// runStdioCommand drives the hidden execution-stdio command itself, the entry
// point the connecting caller launches, with the request on its standard input.
func runStdioCommand(t *testing.T, request string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	executionStdioCmd.SetIn(strings.NewReader(request))
	executionStdioCmd.SetOut(&out)
	executionStdioCmd.SetContext(context.Background())
	t.Cleanup(func() { executionStdioCmd.SetIn(nil); executionStdioCmd.SetOut(nil) })
	err := executionStdioCmd.RunE(executionStdioCmd, nil)
	return out.String(), err
}

const boundaryRequest = `{"version":%V%,"operation":"execute","request":{"ID":"c","WorkingDir":"/synthetic/work","Prompt":"x","Sandbox":{"Mode":"read-only"},"ReadableRoots":["/synthetic/root"],"ReadableRootIdentities":[{"path":"/synthetic/root","device":1,"inode":2}]}}`

// The new reader, reached through the real command: a version it does not
// speak is a typed refusal that echoes neither the root nor the identity.
func TestExecutionStdioCommandRefusesUnknownVersionTyped(t *testing.T) {
	out, err := runStdioCommand(t, strings.Replace(boundaryRequest, "%V%", "5", 1))
	var refusal *unsupportedProtocolVersionError
	if !errors.As(err, &refusal) || refusal.Version != 5 {
		t.Fatalf("refusal is not typed: %T %v", err, err)
	}
	if strings.Contains(err.Error(), "synthetic") || strings.Contains(err.Error(), "inode") || out != "" {
		t.Fatalf("refusal echoes request content: err=%v out=%q", err, out)
	}
}

// requireBoundRootsRefusal asserts the typed v4 refusal came out of the real
// command with the wanted reason, no output, and neither the root path nor an
// identity value in the error.
func requireBoundRootsRefusal(t *testing.T, out string, err error, reason string) {
	t.Helper()
	var refusal *boundRootsRefusalError
	if !errors.As(err, &refusal) || !strings.Contains(refusal.Reason, reason) {
		t.Fatalf("refusal is not the typed bound-roots refusal %q: %T %v", reason, err, err)
	}
	if out != "" || strings.Contains(err.Error(), "synthetic") || strings.Contains(err.Error(), "inode") {
		t.Fatalf("refusal echoes request content or wrote output: err=%v out=%q", err, out)
	}
}

// Every v4 gate, reached through the real command, refuses with the typed error
// rather than dropping the identity fields or serving the request.
func TestExecutionStdioCommandRefusesV4TypedAtEveryGate(t *testing.T) {
	// v3 carrying identities: refused, not served with the identities ignored.
	out, err := runStdioCommand(t, strings.Replace(boundaryRequest, "%V%", "3", 1))
	requireBoundRootsRefusal(t, out, err, "require protocol version 4")
	// v4 with roots but no identities: refused.
	noIdentities := strings.Replace(strings.Replace(boundaryRequest, "%V%", "4", 1), `,"ReadableRootIdentities":[{"path":"/synthetic/root","device":1,"inode":2}]`, "", 1)
	out, err = runStdioCommand(t, noIdentities)
	requireBoundRootsRefusal(t, out, err, "requires an identity for every readable root")
	// v4 with matching identities passes both request gates and is judged on the
	// provider: a CLI provider cannot enforce bound roots, so it is refused
	// typed, and nothing is run.
	prevKind := executionProviderKind
	executionProviderKind = "claude"
	t.Cleanup(func() { executionProviderKind = prevKind })
	out, err = runStdioCommand(t, strings.Replace(boundaryRequest, "%V%", "4", 1))
	requireBoundRootsRefusal(t, out, err, "provider cannot enforce bound readable roots")
}
