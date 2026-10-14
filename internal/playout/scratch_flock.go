//go:build !windows

package playout

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// holdScratch takes the root's owner lock for this process's lifetime.
func holdScratch(root string) (release func(), err error) {
	f, err := os.OpenFile(filepath.Join(root, scratchLock), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}

// scratchAbandoned reports whether no live process owns root: its owner lock can be taken.
func scratchAbandoned(root string) bool {
	f, err := os.Open(filepath.Join(root, scratchLock))
	if errors.Is(err, fs.ErrNotExist) {
		return !writtenSince(root, time.Now().Add(-unlockedQuiet))
	}
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }() // closing drops the probe's lock
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil
}
