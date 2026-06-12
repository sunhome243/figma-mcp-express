//go:build !windows

package internal

import (
	"os"
	"syscall"
)

// lockManifest takes an advisory exclusive flock on the spill manifest so
// concurrent spills can't interleave a partial line. Best-effort: if the lock
// can't be acquired the write still proceeds (a single O_APPEND line write is
// atomic on local filesystems anyway). Returns an unlock func that is always
// safe to defer, even when the lock was not taken.
func lockManifest(f *os.File) func() {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return func() {}
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
}
