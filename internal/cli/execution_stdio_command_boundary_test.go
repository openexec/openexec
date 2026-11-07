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

// A version the reader speaks gets past the version gate and is judged on its
// identities, which it has read rather than dropped.
func TestExecutionStdioCommandDoesNotDropIdentities(t *testing.T) {
	// v3 carrying identities: refused, not served with the identities ignored.
	_, err := runStdioCommand(t, strings.Replace(boundaryRequest, "%V%", "3", 1))
	if err == nil || !strings.Contains(err.Error(), "require protocol version 4") {
		t.Fatalf("v3 with identities: %v", err)
	}
	// v4 with roots but no identities: refused.
	noIdentities := strings.Replace(strings.Replace(boundaryRequest, "%V%", "4", 1), `,"ReadableRootIdentities":[{"path":"/synthetic/root","device":1,"inode":2}]`, "", 1)
	_, err = runStdioCommand(t, noIdentities)
	if err == nil || !strings.Contains(err.Error(), "requires an identity for every readable root") {
		t.Fatalf("v4 without identities: %v", err)
	}
	if strings.Contains(err.Error(), "synthetic") {
		t.Fatalf("refusal echoes root: %v", err)
	}
	// Near miss: v4 with a matching identity is not refused at the version or
	// identity-count gate; any refusal comes from later.
	_, err = runStdioCommand(t, strings.Replace(boundaryRequest, "%V%", "4", 1))
	var refusal *unsupportedProtocolVersionError
	if errors.As(err, &refusal) || (err != nil && strings.Contains(err.Error(), "requires an identity")) {
		t.Fatalf("v4 with matching identities was refused at the gate: %v", err)
	}
}
