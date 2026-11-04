package execution

import (
	"fmt"
	"os"
)

// The platform files answer these; the variables exist so tests can drive the
// real unsupported-platform refusal on a platform that does support identity.
// Nothing outside tests assigns them.
var (
	readableRootIdentitySupported = ReadableRootIdentitySupported
	readableRootIdentityOf        = ReadableRootIdentityOf
)

// unsupportedReadableRootIdentity is the refusal for platforms without a
// verifiable directory identity. It is untagged so every platform compiles it.
func unsupportedReadableRootIdentity(path string, _ os.FileInfo) (ReadableRootIdentity, error) {
	return ReadableRootIdentity{}, fmt.Errorf("readable root identity is unsupported for %q", path)
}
