package execution

import (
	"os"

	"golang.org/x/sys/unix"
)

// darwinRootWatch is a kqueue vnode watch on the root directory's inode: a
// rename, delete, revoke or attribute change of the directory is queued at the
// moment it happens, however briefly it is undone afterwards.
type darwinRootWatch struct {
	kq  int
	dir *os.File
}

func watchPinnedRoot(root *os.Root) rootWatch {
	dir, err := root.Open(".")
	if err != nil {
		return nil
	}
	kq, err := unix.Kqueue()
	if err != nil {
		_ = dir.Close()
		return nil
	}
	var change unix.Kevent_t
	unix.SetKevent(&change, int(dir.Fd()), unix.EVFILT_VNODE, unix.EV_ADD|unix.EV_CLEAR)
	change.Fflags = unix.NOTE_DELETE | unix.NOTE_RENAME | unix.NOTE_REVOKE | unix.NOTE_ATTRIB
	if _, err := unix.Kevent(kq, []unix.Kevent_t{change}, nil, nil); err != nil {
		_ = unix.Close(kq)
		_ = dir.Close()
		return nil
	}
	return &darwinRootWatch{kq: kq, dir: dir}
}

func (w *darwinRootWatch) changed() bool {
	var events [1]unix.Kevent_t
	n, err := unix.Kevent(w.kq, nil, events[:], &unix.Timespec{})
	return err != nil || n > 0
}

func (w *darwinRootWatch) close() {
	_ = unix.Close(w.kq)
	_ = w.dir.Close()
}
