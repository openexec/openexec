//go:build linux || darwin

package execution

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// pinnedRootStillNamed proves, at the moment of use, that the canonical
// absolute path admitted for root still reaches the very directory the root
// descriptor holds. It walks from "/" with O_NOFOLLOW at every component, so a
// symlink or a replacement directory swapped in after admission is refused
// instead of being answered from the old descriptor.
func pinnedRootStillNamed(root *os.Root, path string) error {
	stale := errors.New("the readable root changed after admission")
	if !filepath.IsAbs(path) {
		return stale
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return stale
	}
	for _, component := range strings.Split(strings.Trim(filepath.Clean(path), "/"), "/") {
		if component == "" {
			continue
		}
		next, err := unix.Openat(fd, component, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if err != nil {
			return stale
		}
		fd = next
	}
	defer unix.Close(fd)
	var walked unix.Stat_t
	if unix.Fstat(fd, &walked) != nil {
		return stale
	}
	info, err := root.Stat(".")
	if err != nil {
		return stale
	}
	held, ok := info.Sys().(*syscall.Stat_t)
	if !ok || uint64(held.Dev) != uint64(walked.Dev) || uint64(held.Ino) != uint64(walked.Ino) {
		return stale
	}
	return nil
}

// noProcWalkHook is called immediately before each component is opened by
// openedFileByDescriptorWalk. It is nil outside tests, which use it to
// interleave a swap with the production traversal deterministically.
var noProcWalkHook func(component string)

// openedFileByDescriptorWalk proves that file is the object reached from the
// root descriptor through plain (non-symlink) components only. Each component
// is opened with O_NOFOLLOW relative to the directory descriptor of the one
// before it, so a symlink at any component is refused by the kernel at the
// moment of the open: there is no separate Lstat and Stat for a swap to slip
// between. The final descriptor must be the same device/inode as the file the
// caller opened; otherwise the name was swapped and the check fails closed.
func openedFileByDescriptorWalk(root *os.Root, file *os.File, cleaned string) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	rootFD := int(dir.Fd())
	fd := rootFD
	defer func() {
		if fd != rootFD {
			_ = unix.Close(fd)
		}
	}()
	components := strings.Split(cleaned, string(filepath.Separator))
	for index, component := range components {
		if noProcWalkHook != nil {
			noProcWalkHook(component)
		}
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC
		if index < len(components)-1 {
			flags |= unix.O_DIRECTORY
		} else {
			flags |= unix.O_NONBLOCK
		}
		next, err := unix.Openat(fd, component, flags, 0)
		if err != nil {
			if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
				return errors.New("cannot verify the target of a symbolic link on this platform")
			}
			return errors.New("the path changed while it was being verified")
		}
		if fd != rootFD {
			_ = unix.Close(fd)
		}
		fd = next
	}
	var walked unix.Stat_t
	if err := unix.Fstat(fd, &walked); err != nil {
		return errors.New("the path changed while it was being verified")
	}
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	held, ok := opened.Sys().(*syscall.Stat_t)
	if !ok || uint64(held.Dev) != uint64(walked.Dev) || uint64(held.Ino) != uint64(walked.Ino) {
		return errors.New("the path changed while it was being opened")
	}
	return nil
}
