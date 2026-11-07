package execution

import (
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// linuxRootWatch is an inotify watch on the root directory's inode. The kernel
// queues an event at the moment the directory is renamed, removed or has its
// attributes changed, so a swap that is undone before any name check, inside a
// single timestamp tick, is still reported: nothing here depends on a clock.
type linuxRootWatch struct{ fd int }

// watchPinnedRoot watches the inode the root descriptor holds. It reaches that
// inode through the descriptor (/proc/self/fd), never through a name a swap
// could redirect. It returns nil where that is not possible, and the caller
// falls back to the change-time comparison.
func watchPinnedRoot(root *os.Root) rootWatch {
	dir, err := root.Open(".")
	if err != nil {
		return nil
	}
	defer dir.Close()
	ifd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		return nil
	}
	mask := uint32(unix.IN_MOVE_SELF | unix.IN_DELETE_SELF | unix.IN_ATTRIB | unix.IN_UNMOUNT)
	if _, err := unix.InotifyAddWatch(ifd, "/proc/self/fd/"+strconv.Itoa(int(dir.Fd())), mask); err != nil {
		_ = unix.Close(ifd)
		return nil
	}
	return &linuxRootWatch{fd: ifd}
}

// changed reports whether any event is queued. A failed read other than "no
// event yet" is treated as a change: unproven is refused.
func (w *linuxRootWatch) changed() bool {
	var buf [unix.SizeofInotifyEvent + unix.NAME_MAX + 1]byte
	n, err := unix.Read(w.fd, buf[:])
	if err == unix.EAGAIN {
		return false
	}
	return err != nil || n > 0
}

func (w *linuxRootWatch) close() { _ = unix.Close(w.fd) }
