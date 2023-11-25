//go:build linux

package testutils

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/fuweid/go-dmflakey"

	"github.com/fuweid/go-dmflakey/contrib/sysutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// RequiresRoot exits if the user is not root.
func RequiresRoot() {
	if os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "This test suite must be run as root.")
		os.Exit(1)
	}
}

// RequiresCommands exits if required command is not found.
func RequiresCommands(cmds ...string) {
	for _, cmd := range cmds {
		_, err := exec.LookPath(cmd)
		if err != nil {
			fmt.Fprintln(os.Stderr, fmt.Sprintf("This test suite requires %s command", cmd))
			os.Exit(1)
		}
	}
}

// RequiresCgroupV2 skips if there is no cgroupv2 mount point.
func RequiresCgroupV2(tb testing.TB) {
	_, err := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	if err != nil {
		tb.Skipf("Test %s requires cgroup v2", tb.Name())
	}
}

// FlakeyDevice extends dmflakey.Flakey interface.
type FlakeyDevice interface {
	// RootFS returns root filesystem.
	RootFS() string

	// PowerFailure simulates power failure with drop all the writes.
	PowerFailure(mntOpt string) error

	dmflakey.Flakey
}

// InitFlakeyDevice returns FlakeyDevice instance with a given filesystem.
func InitFlakeyDevice(t *testing.T, name string, fsType dmflakey.FSType, mntOpt string) FlakeyDevice {
	imgDir := t.TempDir()

	flakey, err := dmflakey.InitFlakey(name, imgDir, fsType)
	require.NoError(t, err, "init flakey %s", name)
	t.Cleanup(func() {
		assert.NoError(t, flakey.Teardown())
	})

	rootDir := t.TempDir()
	err = unix.Mount(flakey.DevicePath(), rootDir, string(fsType), 0, mntOpt)
	require.NoError(t, err, "init rootfs on %s", rootDir)

	t.Cleanup(func() { assert.NoError(t, sysutils.UnmountAll(rootDir, 0)) })

	return &flakeyT{
		Flakey: flakey,

		rootDir: rootDir,
		mntOpt:  mntOpt,
	}
}

type flakeyT struct {
	dmflakey.Flakey

	rootDir string
	mntOpt  string
}

// RootFS returns root filesystem.
func (f *flakeyT) RootFS() string {
	return f.rootDir
}

// PowerFailure simulates power failure with drop all the writes.
func (f *flakeyT) PowerFailure(mntOpt string) error {
	if err := f.DropWrites(); err != nil {
		return fmt.Errorf("failed to drop_writes: %w", err)
	}

	if err := sysutils.UnmountAll(f.rootDir, 0); err != nil {
		return fmt.Errorf("failed to unmount rootfs %s: %w", f.rootDir, err)
	}

	if mntOpt == "" {
		mntOpt = f.mntOpt
	}

	if err := f.AllowWrites(); err != nil {
		return fmt.Errorf("failed to allow_writes: %w", err)
	}

	if err := unix.Mount(f.DevicePath(), f.rootDir, string(f.Filesystem()), 0, mntOpt); err != nil {
		return fmt.Errorf("failed to mount rootfs %s: %w", f.rootDir, err)
	}
	return nil
}
