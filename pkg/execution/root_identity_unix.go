//go:build linux || darwin

package execution

import (
	"fmt"
	"os"
	"syscall"
)

func ReadableRootIdentitySupported() bool { return true }

// ReadableRootIdentityOf extracts the stable device/inode identity from an
// already opened directory. It never follows a model-provided path.
func ReadableRootIdentityOf(path string, info os.FileInfo) (ReadableRootIdentity, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() {
		return ReadableRootIdentity{}, fmt.Errorf("directory identity unavailable for %q", path)
	}
	return ReadableRootIdentity{Path: path, Device: uint64(stat.Dev), Inode: uint64(stat.Ino)}, nil
}
