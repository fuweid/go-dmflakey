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

	"github.com/containerd/cgroups/v3/cgroup2"
	"github.com/fuweid/go-dmflakey/contrib/utils"

	"github.com/fuweid/go-dmflakey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var defaultCgroup2Path = "/sys/fs/cgroup"

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

	_, err := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	if err != nil {
		fmt.Fprintln(os.Stderr, "This test requires cgroupv2")
		os.Exit(1)
	}
}

func TestPullImageWithSyncFs(t *testing.T) {
	imageName := "localhost:5000/golang:1.19.4"

	t.Logf("Move current proc into cgroupv2")
	subtree := "/" + strings.ToLower(t.Name())
	mgr, err := cgroup2.NewManager(defaultCgroup2Path, subtree, &cgroup2.Resources{})
	require.NoError(t, err)

	rootMgr, err := cgroup2.NewManager(defaultCgroup2Path, "/", &cgroup2.Resources{})
	require.NoError(t, err)
	require.NoError(t, mgr.AddProc(uint64(os.Getpid())))
	defer func() {
		require.NoError(t, mgr.MoveTo(rootMgr))
	}()

	flakey, root := initFlakeyDevice(t, dmflakey.FSTypeEXT4)

	/*
		t.Logf("Use fast_commit to ext4")
		_, err = exec.Command("tune2fs", "-O", "fast_commit", flakey.DevicePath()).CombinedOutput()
		require.NoError(t, err)
	*/

	// NOTE: Set commit=1000 is to ensure that the global writeback won't
	// persist all the data during the simulation of power failure. So,
	// if the process doesn't call fsync/fdatasync, the data won't be committed
	// into disk.
	//
	// REF:
	// Query about ext4 commit interval vs dirty_expire_centisecs - https://lore.kernel.org/linux-ext4/20191213155912.GH15474@quack2.suse.cz/
	require.NoError(t, utils.Mount(root, flakey.DevicePath(), "commit=1000"))

	logPath := filepath.Join(t.TempDir(), "containerd.log")
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	sockAddr := filepath.Join(root, "run", "containerd", "containerd.sock")

	require.NoError(t,
		os.WriteFile(cfgPath, []byte(`
version = 2

[plugins]
  [plugins.'io.containerd.grpc.v1.cri']
    image_pull_with_sync_fs = true
`),
			0600),
	)

	var runContainerd = func() (chan *exec.Cmd, chan error) {
		logFd, err := os.Create(logPath)
		require.NoError(t, err)

		cmdCh, waitCh := make(chan *exec.Cmd), make(chan error)
		go func() {
			defer logFd.Close()
			defer close(waitCh)

			args := []string{
				"containerd",
				"--config", cfgPath,
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
	defer ctrdCmd.Process.Kill()

	t.Logf("Get the device number")
	st, err := os.Stat(filepath.Join(root, "var", "lib", "containerd"))
	require.NoError(t, err)
	stat := st.Sys().(*syscall.Stat_t)
	major, minor := stat.Dev>>8, stat.Dev&(1<<8-1)

	start := time.Now()
	runCommand(t, "crictl", "-r", sockAddr, "pull", imageName)
	t.Logf("Take %v to pull %s", time.Since(start), imageName)

	ctrdCmd.Process.Kill()
	require.Error(t, <-waitCh)

	t.Logf("Show the blkio status")
	m, err := mgr.Stat()
	require.NoError(t, err)

	for _, entry := range m.Io.Usage {
		if entry.Major == major && entry.Minor == minor {
			t.Logf("WIOS=%d, WBytes=%d, RIOS=%d, RBytes=%d",
				entry.Wios, entry.Wbytes, entry.Rios, entry.Rbytes)
		}
	}
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
