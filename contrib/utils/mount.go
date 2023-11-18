//go:build linux

package utils

import (
	"fmt"
	"os/exec"
	"time"

	"golang.org/x/sys/unix"
)

// Mount mounts devPath on target with option.
func Mount(target string, devPath string, opt string) error {
	args := []string{"-o", opt, devPath, target}

	output, err := exec.Command("mount", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to mount (args: %v) (out: %s): %w",
			args, string(output), err)
	}
	return nil
}

// Unmount umounts target mount point.
func Unmount(target string) error {
	for i := 0; i < 50; i++ {
		if err := unix.Unmount(target, 0); err != nil {
			switch err {
			case unix.EBUSY:
				time.Sleep(500 * time.Millisecond)
				continue
			case unix.EINVAL:
			default:
				return fmt.Errorf("failed to umount %s: %w", target, err)
			}
		}
		return nil
	}
	return unix.EBUSY
}
