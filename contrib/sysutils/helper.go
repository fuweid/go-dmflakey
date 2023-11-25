//go:build linux

package sysutils

import (
	"fmt"
	"os"
	"time"

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

// UnmountAll umounts target mount point recursively.
func UnmountAll(target string, flags int) error {
	for i := 0; i < 50; i++ {
		if err := unix.Unmount(target, flags); err != nil {
			switch err {
			case unix.EBUSY:
				time.Sleep(500 * time.Millisecond)
				continue
			case unix.EINVAL:
				return nil
			default:
				return fmt.Errorf("failed to umount %s: %w", target, err)
			}
		}
		continue
	}
	return fmt.Errorf("failed to umount %s: %w", target, unix.EBUSY)
}
