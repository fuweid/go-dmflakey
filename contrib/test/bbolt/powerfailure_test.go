//go:build linux

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fuweid/go-dmflakey"
	"github.com/fuweid/go-dmflakey/contrib/sysutils"
	"github.com/fuweid/go-dmflakey/contrib/testutils"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

// TestDropWritesDuringBench is used to drop_writes during bbolt bench. It's
// used to cause data loss during power failure.
func TestDropWritesDuringBench(t *testing.T) {
	flakey := testutils.InitFlakeyDevice(t, t.Name(), dmflakey.FSTypeEXT4, "")

	rootfs := flakey.RootFS()
	dbPath := filepath.Join(rootfs, "boltdb")

	// NOTE: Creates database with large size to make sure there is no file's
	// The flakey device handles IO in block layer, which means that it's
	// unlikely to distinguish the content between file's and filesystem's
	// metadata. So, using large size is to ensure that there is no fsync
	// syscall called from bbolt-bench command. All the data synced is
	// triggered by fdatasync. So, we can ensure all the dropped data is
	// from file. Otherwise, it's easy to break filesystem.
	t.Logf("Init empty bbolt database with 128 MiB")
	initEmptyBoltdb(t, dbPath, 128*1024*1024)
	defer dumpDBIfFailed(t, dbPath, false)

	// Ensure all the data has been persisted in the device.
	t.Logf("Ensure the empty boltdb data persisted in the flakey device")
	sysutils.SyncFS(dbPath)

	args := []string{
		"bench",
		"-work", // keep the database
		"-path", dbPath,
		"-count=1000000000",
		"-batch-size=5", // separate total count into multiple truncation
	}

	t.Logf("Starting to run bbolt %s", strings.Join(args, " "))

	bboltCmd := exec.Command("bbolt", args...)
	var logBuf bytes.Buffer
	bboltCmd.Stdout, bboltCmd.Stderr = &logBuf, &logBuf
	defer func() {
		if t.Failed() {
			t.Logf("Dumping bbolt log:\n%s", logBuf.String())
		}
	}()
	require.NoError(t, bboltCmd.Start(), "start bbolt (args: %s)", strings.Join(args, " "))

	t.Logf("Keep bbolt-bench running for 3 seconds")
	time.Sleep(3 * time.Second)

	t.Logf("Kill the bbolt process and simulate power failure")
	bboltCmd.Process.Kill()
	require.Error(t, bboltCmd.Wait())
	require.NoError(t, flakey.PowerFailure(""))

	t.Logf("Invoke bbolt check to verify data")
	if output, err := exec.Command("bbolt", "check", dbPath).CombinedOutput(); err != nil {
		t.Errorf("bbolt check failed, error: %v, output: %s", err, string(output))
	}
}

// dumpDBIfFailed dumps database into env RESULTS_DIR or /tmp/XXXX.
func dumpDBIfFailed(t *testing.T, dbPath string, force bool) {
	if !t.Failed() && !force {
		return
	}

	t.Log("Init result dir before dump database")

	var err error
	resultDir, _ := os.LookupEnv("RESULTS_DIR")
	if resultDir == "" {
		resultDir, err = os.MkdirTemp("", "*.db")
		require.NoError(t, err)
	}

	resultDir, err = filepath.Abs(filepath.Join(resultDir, strings.ReplaceAll(t.Name(), "/", "_")))
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(resultDir))
	require.NoError(t, os.MkdirAll(resultDir, 0600))

	targetDbPath := filepath.Join(resultDir, "dump.db")
	t.Logf("Dumping database %s into %s", dbPath, targetDbPath)
	require.NoError(t, copyFile(dbPath, targetDbPath))
}

// initEmptyBoltdb inits empty boltdb with size.
func initEmptyBoltdb(t *testing.T, dbPath string, size int64) {
	_, err := os.Stat(dbPath)
	require.True(t, errors.Is(err, os.ErrNotExist))

	db, err := bbolt.Open(dbPath, 0600, nil)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	dbFd, err := os.OpenFile(dbPath, os.O_RDWR, 0600)
	require.NoError(t, err)
	defer dbFd.Close()

	require.NoError(t, dbFd.Truncate(size))
	require.NoError(t, dbFd.Sync())
}

// copyFile copies source file into dst.
func copyFile(src, dst string) error {
	srcFd, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFd.Close()

	dstFd, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFd.Close()

	_, err = io.Copy(dstFd, srcFd)
	if err == io.EOF {
		err = dstFd.Sync()
	}
	return err
}
