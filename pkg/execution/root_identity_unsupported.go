//go:build !linux && !darwin

package execution

import "os"

func ReadableRootIdentitySupported() bool { return false }

// Other platforms must not pretend a path is a verified directory identity.
func ReadableRootIdentityOf(path string, info os.FileInfo) (ReadableRootIdentity, error) {
	return unsupportedReadableRootIdentity(path, info)
}
