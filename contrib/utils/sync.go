//go:build linux

package utils

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// SyncFS synchronizes the filesystem containing file.
//
// REF: https://man7.org/linux/man-pages/man2/syncfs.2.html
func SyncFS(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", file, err)
	}
	defer f.Close()

	_, _, errno := unix.Syscall(unix.SYS_SYNCFS, uintptr(f.Fd()), 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
