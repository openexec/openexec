//go:build darwin

package execution

import "syscall"

func statChangeStamp(s *syscall.Stat_t) int64 {
	return int64(s.Ctimespec.Sec)*1_000_000_000 + int64(s.Ctimespec.Nsec)
}
