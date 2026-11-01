//go:build !linux && !darwin

package execution

import (
	"fmt"
	"os"
)

func ReadableRootIdentitySupported() bool { return false }

// Other platforms must not pretend a path is a verified directory identity.
func ReadableRootIdentityOf(path string, _ os.FileInfo) (ReadableRootIdentity, error) {
	return ReadableRootIdentity{}, fmt.Errorf("readable root identity is unsupported for %q", path)
}
