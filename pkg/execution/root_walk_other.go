//go:build !linux && !darwin

package execution

import (
	"errors"
	"os"
)

var noProcWalkHook func(component string)

// openedFileByDescriptorWalk has no directory-descriptor walk on this
// platform, so verification without a kernel name fails closed.
func openedFileByDescriptorWalk(_ *os.Root, _ *os.File, _ string) error {
	return errors.New("cannot verify the opened path on this platform")
}

type rootWatch interface {
	changed() bool
	close()
}

func watchPinnedRoot(_ *os.Root) rootWatch { return nil }

func pinnedRootStillNamed(_ *os.Root, _ string) error {
	return errors.New("cannot verify the readable root on this platform")
}
