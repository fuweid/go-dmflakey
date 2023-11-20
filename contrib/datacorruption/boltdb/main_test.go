//go:build linux

package main

import (
	"bytes"
	"errors"
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

		var b bytes.Buffer
		cmd.Stdout = &b
		cmd.Stderr = &b

		require.NoError(t, cmd.Start(), "start bbolt (args: %v)", args[1:])
		werr := cmd.Wait()
		t.Logf("bbolt output: \n %s\n", b.String())
		waitCh <- werr
	}()

	cmd := <-cmdCh

	t.Logf("Keep bbolt-bench running for 3 seconds")
	time.Sleep(3 * time.Second)

	defer func() {
		t.Log("Save data if failed")
		saveDataIfFailed(t, dbPath, false)
	}()

	t.Logf("Kill the bbolt process and simulate power failure")
	cmd.Process.Kill()
	require.Error(t, <-waitCh)
	require.NoError(t, utils.SimulatePowerFailure(flakey, root))

	t.Logf("Invoke bbolt check to verify data")
	if output, err := exec.Command("bbolt", "check", dbPath).CombinedOutput(); err != nil {
		t.Errorf("bbolt check failed, error: %v, output: %s", err, string(output))
	}
}

func saveDataIfFailed(t *testing.T, dbFilePath string, force bool) {
	if t.Failed() || force {
		t.Log("Backup bbolt db file...")

		backupDir := testResultsDirectory(t)
		backupDB(t, dbFilePath, backupDir)
	}
}

func testResultsDirectory(t *testing.T) string {
	resultsDirectory, ok := os.LookupEnv("RESULTS_DIR")
	var err error
	if !ok {
		resultsDirectory, err = os.MkdirTemp("", "*.db")
		require.NoError(t, err)
	}
	resultsDirectory, err = filepath.Abs(resultsDirectory)
	require.NoError(t, err)

	path, err := filepath.Abs(filepath.Join(resultsDirectory, strings.ReplaceAll(t.Name(), "/", "_")))
	require.NoError(t, err)

	err = os.RemoveAll(path)
	require.NoError(t, err)

	err = os.MkdirAll(path, 0700)
	require.NoError(t, err)

	return path
}

func backupDB(t *testing.T, srcFilePath string, dstDir string) {
	dstFilePath := filepath.Join(dstDir, "db.bak")
	t.Logf("Saving the DB file to %s", dstFilePath)
	output, err := exec.Command("cp", srcFilePath, dstFilePath).CombinedOutput()
	require.NoError(t, err, "cp db file from %q to %q failed: %s", srcFilePath, dstFilePath, string(output))
	t.Logf("DB file saved to %s", dstFilePath)
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
