//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/fuweid/go-dmflakey/contrib/sysutils"
	"github.com/fuweid/go-dmflakey/contrib/testutils"

	"github.com/containerd/cgroups/v3/cgroup2"
	"github.com/fuweid/go-dmflakey"
	"github.com/stretchr/testify/require"
)

var defaultCgroup2Path = "/sys/fs/cgroup"

// TestIssue5854 is used to test data integrity after power failure.
//
// It's to reproduce issue https://github.com/containerd/containerd/issues/5854.
//
// NOTE: Please ensure that containerd binary doesn't include the fix, like
//
//	https://github.com/containerd/containerd/pull/9401
func TestIssue5854(t *testing.T) {
	// NOTE: Set commit=1000 is to ensure that the global writeback won't
	// persist all the data during the simulation of power failure. So,
	// if the process doesn't call fsync/fdatasync, the data won't be
	// committed into disk.
	//
	// REF:
	// Query about ext4 commit interval vs dirty_expire_centisecs - https://lore.kernel.org/linux-ext4/20191213155912.GH15474@quack2.suse.cz/
	flakey := testutils.InitFlakeyDevice(t, t.Name(), dmflakey.FSTypeEXT4, "commit=1000")

	rootfs := flakey.RootFS()

	logPath := filepath.Join(t.TempDir(), "containerd.log")
	sockAddr := filepath.Join(rootfs, "run", "containerd", "containerd.sock")

	t.Log("Start to run containerd")
	ctrdCmd := runContainerd(t, rootfs, "", logPath)

	t.Log("Wait for ready")
	expectLog(t, logPath, "containerd successfully booted in")

	imageName := "ghcr.io/containerd/alpine:3.14.0"
	t.Logf("Pulling %s", imageName)
	runCommand(t, "crictl", "-r", sockAddr, "pull", imageName)

	t.Log("Power failure")
	ctrdCmd.Process.Kill()
	require.Error(t, ctrdCmd.Wait())
	require.NoError(t, flakey.PowerFailure(""))

	t.Log("Restarting containerd")
	ctrdCmd = runContainerd(t, rootfs, "", logPath)

	t.Log("Wait for ready")
	expectLog(t, logPath, "containerd successfully booted in")
	defer ctrdCmd.Process.Kill()

	targetMount := filepath.Join(t.TempDir())
	defer sysutils.UnmountAll(targetMount, 0)

	t.Logf("Mounting image %s on %s", imageName, targetMount)
	runCommand(t, "ctr", "-a", sockAddr, "-n", "k8s.io", "image", "mount", imageName, targetMount)

	t.Logf("Run busybox (Chroot: %s)", targetMount)
	busyboxCmd := exec.Command(filepath.Join("/bin", "busybox"))
	busyboxCmd.SysProcAttr = &syscall.SysProcAttr{Chroot: targetMount}
	_, err := busyboxCmd.CombinedOutput()
	require.ErrorContains(t, err, "/bin/busybox: exec format error")
	t.Log(err.Error())
}

// TestPull9401 is only used for https://github.com/containerd/containerd/pull/9401.
func TestPull9401(t *testing.T) {
	testutils.RequiresCgroupV2(t)

	flakey := testutils.InitFlakeyDevice(t, t.Name(), dmflakey.FSTypeEXT4, "")
	rootfs := flakey.RootFS()

	logPath := filepath.Join(t.TempDir(), "containerd.log")
	sockAddr := filepath.Join(rootfs, "run", "containerd", "containerd.sock")

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t,
		os.WriteFile(cfgPath, []byte(`
version = 2

[plugins]
  [plugins.'io.containerd.grpc.v1.cri']
    image_pull_with_sync_fs = false
`),
			0600),
	)

	t.Log("Start to run containerd")
	ctrdCmd := runContainerd(t, rootfs, cfgPath, logPath)

	t.Log("Wait for ready")
	expectLog(t, logPath, "containerd successfully booted in")

	t.Logf("Move containerd proc into cgroupv2")
	rootCgMgr, err := cgroup2.NewManager(defaultCgroup2Path, "/", &cgroup2.Resources{})
	require.NoError(t, err)

	subtree := "/" + strings.ToLower(t.Name())
	cgMgr, err := cgroup2.NewManager(defaultCgroup2Path, subtree, &cgroup2.Resources{})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cgMgr.MoveTo(rootCgMgr))
	}()

	require.NoError(t, cgMgr.AddProc(uint64(ctrdCmd.Process.Pid)))

	t.Logf("Get the device number")
	major, minor := getDeviceMajorAndMinor(t, filepath.Join(rootfs, "var", "lib", "containerd"))

	// NOTE: Change if there is no such image in your local
	imageName := "localhost:5000/golang:1.19.4"

	start := time.Now()
	runCommand(t, "crictl", "-r", sockAddr, "pull", imageName)
	t.Logf("Take %v to pull %s", time.Since(start), imageName)

	ctrdCmd.Process.Kill()
	require.Error(t, ctrdCmd.Wait())

	t.Logf("Show the blkio status")
	m, err := cgMgr.Stat()
	require.NoError(t, err)

	for _, entry := range m.Io.Usage {
		if entry.Major == major && entry.Minor == minor {
			t.Logf("WIOS=%d, WBytes=%d, RIOS=%d, RBytes=%d",
				entry.Wios, entry.Wbytes, entry.Rios, entry.Rbytes)
		}
	}
}

// runContainerd runs containerd which listens on $rootfs/run/containerd/containerd.sock.
func runContainerd(t *testing.T, rootfs, cfgPath, logPath string) *exec.Cmd {
	logFd, err := os.Create(logPath)
	require.NoError(t, err, "create containerd log file %s", logPath)
	t.Cleanup(func() { logFd.Close() })

	args := []string{
		"--root", filepath.Join(rootfs, "var", "lib", "containerd"),
		"--state", filepath.Join(rootfs, "run", "containerd"),
		"--address", filepath.Join(rootfs, "run", "containerd", "containerd.sock"),
		"--log-level", "debug",
	}

	if cfgPath != "" {
		args = append(args, "--config", cfgPath)
	}

	cmd := exec.Command("containerd", args...)
	cmd.Stdout, cmd.Stderr = logFd, logFd
	require.NoError(t, cmd.Start(), "start containerd (args: %s)", strings.Join(args, " "))
	return cmd
}

// runCommand runs short-live command.
func runCommand(t *testing.T, command string, args ...string) []byte {
	data, err := exec.Command(command, args...).CombinedOutput()
	output := fmt.Sprintf("%s (args: %s) output:\n %s\n", command, strings.Join(args, " "), string(data))
	require.NoError(t, err, output)
	return data
}

// expectLog expects log content has expected string.
func expectLog(t *testing.T, logPath string, expected string) {
	for i := 0; i < 10; i++ {
		data, err := os.ReadFile(logPath)
		require.NoError(t, err)

		if strings.Contains(string(data), expected) {
			return
		}
		time.Sleep(1 * time.Second)
	}
}

// getDeviceMajorAndMinor returns the device's major and minor.
func getDeviceMajorAndMinor(t *testing.T, file string) (uint64, uint64) {
	st, err := os.Stat(file)
	require.NoError(t, err)
	stat := st.Sys().(*syscall.Stat_t)
	return stat.Dev >> 8, stat.Dev & (1<<8 - 1)
}
