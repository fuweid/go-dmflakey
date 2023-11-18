//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/fuweid/go-dmflakey/contrib/utils"

	"github.com/fuweid/go-dmflakey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
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

	_, err := exec.LookPath("bbolt")
	if err != nil {
		fmt.Fprintln(os.Stderr, "This test requires bbolt command")
		os.Exit(1)
	}
}

// TestDropWritesDuringBench is used to drop_writes during bbolt bench. It's
// used to cause data loss during power failure.
func TestDropWritesDuringBench(t *testing.T) {
	flakey, root := initFlakeyDevice(t, dmflakey.FSTypeEXT4)

	require.NoError(t, utils.Mount(root, flakey.DevicePath(), ""))

	dbPath := filepath.Join(root, "boltdb")

	// NOTE: Creates database with large size to make sure there is no file's
	// The flakey device handles IO in block layer, which means that it's
	// unlikely to distinguish the content between file's and filesystem's
	// metadata. So, using large size is to ensure that there is no fsync
	// syscall called from bbolt-bench command. All the data synced is
	// triggered by fdatasync. So, we can ensure all the dropped data is
	// from file. Otherwise, it's easy to break filesystem.
	t.Logf("Init empty bbolt database with 128 MiB")
	initEmptyBoltdb(t, dbPath)

	// Ensure all the data has been persisted in the device.
	t.Logf("Ensure the empty boltdb data persisted in the flakey device")
	utils.SyncFS(dbPath)

	t.Logf("Start to run bbolt-bench")
	cmdCh, waitCh := make(chan *exec.Cmd), make(chan error)
	// run bbolt bench
	go func() {
		defer close(waitCh)

		args := []string{"bbolt", "bench",
			"-work", // keep the database
			"-path", dbPath,
			"-count=1000000000",
			"-batch-size=5", // separate total count into multiple truncation
		}

		cmd := exec.Command(args[0], args[1:]...)
		cmdCh <- cmd

		require.NoError(t, cmd.Start(), "start bbolt (args: %v)", args[1:])
		waitCh <- cmd.Wait()
	}()

	cmd := <-cmdCh

	t.Logf("Drop all the write IOs after 3 seconds")
	time.Sleep(3 * time.Second)
	require.NoError(t, flakey.DropWrites())

	t.Logf("Let bbolt-bench run with DropWrites mode for 3 seconds")
	time.Sleep(3 * time.Second)

	t.Logf("Start to allow all the write IOs for 2 seconds")
	require.NoError(t, flakey.AllowWrites())
	time.Sleep(2 * time.Second)

	t.Logf("Kill the bbolt process and simulate power failure")
	cmd.Process.Kill()
	require.Error(t, <-waitCh)
	require.NoError(t, simulatePowerFailure(flakey, root))

	t.Logf("Invoke bbolt check to verify data")
	output, err := exec.Command("bbolt", "check", dbPath).CombinedOutput()
	require.NoError(t, err, "bbolt check output: %s", string(output))
}

// initEmptyBoltdb inits empty boltdb with 128 MiB.
func initEmptyBoltdb(t *testing.T, dbPath string) {
	_, err := os.Stat(dbPath)
	require.True(t, errors.Is(err, os.ErrNotExist))

	db, err := bbolt.Open(dbPath, 0600, nil)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	dbFd, err := os.OpenFile(dbPath, os.O_RDWR, 0600)
	require.NoError(t, err)
	defer dbFd.Close()

	require.NoError(t, dbFd.Truncate(128*1024*1024))
	require.NoError(t, dbFd.Sync())
}

func simulatePowerFailure(flakey dmflakey.Flakey, root string) error {
	if err := flakey.DropWrites(); err != nil {
		return err
	}

	if err := utils.Unmount(root); err != nil {
		return err
	}

	if err := flakey.AllowWrites(); err != nil {
		return err
	}
	return utils.Mount(root, flakey.DevicePath(), "")
}

func initFlakeyDevice(t *testing.T, fsType dmflakey.FSType) (_ dmflakey.Flakey, root string) {
	tmpDir := t.TempDir()

	target := filepath.Join(tmpDir, "root")
	require.NoError(t, os.MkdirAll(target, 0600))

	flakey, err := dmflakey.InitFlakey("boltdb", tmpDir, fsType)
	require.NoError(t, err, "init flakey")

	t.Cleanup(func() {
		assert.NoError(t, utils.Unmount(target))
		assert.NoError(t, flakey.Teardown())
	})
	return flakey, target
}
