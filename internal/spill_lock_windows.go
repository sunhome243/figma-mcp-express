//go:build windows

package internal

import "os"

// lockManifest is a no-op on Windows: there is no syscall.Flock, and the spill
// manifest lock is best-effort by design. A single O_APPEND line write is atomic
// on local filesystems, so skipping the advisory lock only risks a rare torn
// line under heavy concurrent spilling — acceptable for a recovery-aid manifest.
// Returns a no-op unlock so the call site stays uniform across platforms.
func lockManifest(f *os.File) func() { return func() {} }
