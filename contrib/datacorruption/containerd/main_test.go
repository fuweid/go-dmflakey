//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fuweid/go-dmflakey/contrib/utils"

	"github.com/fuweid/go-dmflakey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	requires()
	os.Exit(m.Run())
}

func requires() {
	if os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "This test must be run as root.")
		os.Exit(1)
	}

	for _, cmd := range []string{"containerd", "ctr", "crictl"} {
		_, err := exec.LookPath(cmd)
		if err != nil {
			fmt.Fprintln(os.Stderr, fmt.Sprintf("This test requires %s command", cmd))
			os.Exit(1)
		}
	}
}

// TestPowerFailureAfterPullImage is used to test data integrity after power
// failure.
//
// It's to reproduce issue https://github.com/containerd/containerd/issues/5854.
func TestPowerFailureAfterPullImage(t *testing.T) {
	flakey, root := initFlakeyDevice(t, dmflakey.FSTypeEXT4)

	// NOTE: Set commit=1000 is to ensure that the global writeback won't
	// persist all the data during the simulation of power failure. So,
	// if the process doesn't call fsync/fdatasync, the data won't be committed
	// into disk.
	//
	// REF:
	// Query about ext4 commit interval vs dirty_expire_centisecs - https://lore.kernel.org/linux-ext4/20191213155912.GH15474@quack2.suse.cz/
	require.NoError(t, utils.Mount(root, flakey.DevicePath(), "commit=1000"))

	logPath := filepath.Join(t.TempDir(), "containerd.log")
	sockAddr := filepath.Join(root, "run", "containerd", "containerd.sock")

	var runContainerd = func() (chan *exec.Cmd, chan error) {
		logFd, err := os.Create(logPath)
		require.NoError(t, err)

		cmdCh, waitCh := make(chan *exec.Cmd), make(chan error)
		go func() {
			defer logFd.Close()
			defer close(waitCh)

			args := []string{"containerd",
				"--root", filepath.Join(root, "var", "lib", "containerd"),
				"--state", filepath.Join(root, "run", "containerd"),
				"--address", sockAddr,
				"--log-level", "debug",
			}

			cmd := exec.Command(args[0], args[1:]...)
			cmd.Stdout, cmd.Stderr = logFd, logFd
			cmdCh <- cmd

			require.NoError(t, cmd.Start(), "start containerd (args: %v)", args[1:])
			waitCh <- cmd.Wait()
		}()
		return cmdCh, waitCh
	}

	t.Log("Start to run containerd")
	cmdCh, waitCh := runContainerd()
	ctrdCmd := <-cmdCh

	t.Log("Wait for ready")
	expectLog(t, logPath, "containerd successfully booted in")

	imageName := "ghcr.io/containerd/alpine:3.14.0"
	t.Logf("Pulling %s", imageName)
	runCommand(t, "crictl", "-r", sockAddr, "pull", imageName)

	t.Log("Power failure")
	ctrdCmd.Process.Kill()
	require.Error(t, <-waitCh)
	require.NoError(t, utils.SimulatePowerFailure(flakey, root))

	t.Log("Restarting containerd")
	cmdCh, waitCh = runContainerd()

	ctrdCmd = <-cmdCh

	t.Log("Wait for ready")
	expectLog(t, logPath, "containerd successfully booted in")
	defer ctrdCmd.Process.Kill()

	targetMount := filepath.Join(t.TempDir())
	defer utils.Unmount(targetMount)

	t.Logf("Mounting image %s on %s", imageName, targetMount)
	runCommand(t, "ctr", "-a", sockAddr, "-n", "k8s.io", "image", "mount", imageName, targetMount)

	t.Log("Run busybox")
	runCommand(t, filepath.Join(targetMount, "bin", "busybox"))
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

// runCommand runs short-live command.
func runCommand(t *testing.T, command string, args ...string) []byte {
	data, err := exec.Command(command, args...).CombinedOutput()
	output := fmt.Sprintf("%s (args: %v) output:\n %s\n", command, args, string(data))
	require.NoError(t, err, output)
	t.Log(output)

	return data
}

func initFlakeyDevice(t *testing.T, fsType dmflakey.FSType) (_ dmflakey.Flakey, root string) {
	tmpDir := t.TempDir()

	target := filepath.Join(tmpDir, "root")
	require.NoError(t, os.MkdirAll(target, 0600))

	flakey, err := dmflakey.InitFlakey("containerd", tmpDir, fsType)
	require.NoError(t, err, "init flakey")

	t.Cleanup(func() {
		assert.NoError(t, utils.Unmount(target))
		assert.NoError(t, flakey.Teardown())
	})
	return flakey, target
}
