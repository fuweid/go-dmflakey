## go-dmflakey

Go bindings for the [dm-flakey][dm-flakey] device.

### Key Features

* `AllowWrites`: The device is available for `<up interval>` seconds.

* `DropWrites`: All write I/O is silently ignored for `<down interval>` seconds. Read I/O is handled correctly.

* `ErrorWrites`: All write I/O is failed with an error signalled for `<down interval>` seconds. Read I/O is handled correctly.

Both `<up>` and `<down>` interval can be configured by `WithIntervalFeatOpt(interval)`.

TODO: `ErrorReads`, `CorruptBIOByte`, `RandomReadCorrupt`, `RandomWriteCorrupt`.

### Example

* Simulate power failure and cause data loss

```go
// init flakey device named by /dev/mapper/go-dmflakey with empty ext4 filesystem
flakey, _ := InitFlakey("go-dmflakey", workDir, FSTypeEXT4)

flakeyDevice := flakey.DevicePath()

// mount that flakey device into temp dir
args := []string{"mount", flakeyDevice, targetDir}
exec.Command(args[0], args[1:]).Run()

// write file in targetDir without syncfs/fsync/fdatasync
// ...

// drop all write IO created by previous step
flakey.DropWrites()

// remount and allow all write IO to simulate power failure
exec.Command("umount", targetDir)
flakey.AllowWrites()
exec.Command(args[0], args[1:]).Run()

// check the file
```

* How to cause data loss during `bbolt bench`? from contrib [datacorruption-boltdb]

```bash
$ cd contrib

$ go test -c -o /tmp/test ./datacorruption/boltdb && sudo /tmp/test -test.v
=== RUN   TestDropWritesDuringBench
    main_test.go:56: Init empty bbolt database with 128 MiB
    main_test.go:60: Ensure the empty boltdb data persisted in the flakey device
    main_test.go:63: Start to run bbolt-bench
    main_test.go:85: Drop all the write IOs after 3 seconds
    main_test.go:89: Let bbolt-bench run with DropWrites mode for 3 seconds
    main_test.go:92: Start to allow all the write IOs for 2 seconds
    main_test.go:96: Kill the bbolt process and simulate power failure
    main_test.go:101: Invoke bbolt check to verify data
    main_test.go:103:
                Error Trace:    /home/fuwei/workspace/go-dmflakey/contrib/datacorruption/boltdb/main_test.go:103
                Error:          Received unexpected error:
                                exit status 2
                Test:           TestDropWritesDuringBench
                Messages:       bbolt check output: panic: invalid freelist page: 0, page type is unknown<00>

                                goroutine 1 [running]:
                                go.etcd.io/bbolt.(*freelist).read(0x0?, 0x0?)
                                        /home/fuwei/workspace/bbolt/freelist.go:270 +0x199
                                go.etcd.io/bbolt.(*DB).loadFreelist.func1()
                                        /home/fuwei/workspace/bbolt/db.go:400 +0xc5
                                sync.(*Once).doSlow(0xc0001301c0?, 0x584020?)
                                        /usr/local/go/src/sync/once.go:74 +0xc2
                                sync.(*Once).Do(...)
                                        /usr/local/go/src/sync/once.go:65
                                go.etcd.io/bbolt.(*DB).loadFreelist(0xc000130000?)
                                        /home/fuwei/workspace/bbolt/db.go:393 +0x47
                                go.etcd.io/bbolt.Open({0x7ffc60ce9530, 0x38}, 0x670060?, 0xc00005fc18)
                                        /home/fuwei/workspace/bbolt/db.go:275 +0x44f
                                main.(*checkCommand).Run(0xc000137e58, {0xc0000161a0, 0x1, 0x1})
                                        /home/fuwei/workspace/bbolt/cmd/bbolt/main.go:212 +0x1e5
                                main.(*Main).Run(0xc00005ff40, {0xc000016190?, 0xc0000061a0?, 0x200000003?})
                                        /home/fuwei/workspace/bbolt/cmd/bbolt/main.go:124 +0x4d4
                                main.main()
                                        /home/fuwei/workspace/bbolt/cmd/bbolt/main.go:62 +0xae
--- FAIL: TestDropWritesDuringBench (8.29s)
FAIL
```

* What if power failure after pulling image? from contrib [datacorruption-containerd]

It's reproducer for [container not starting in few nodes "standard_init_linux.go:219: exec user process caused: exec format error"](https://github.com/containerd/containerd/issues/5854).

```bash
$ cd contrib

$ go test -c -o /tmp/test ./datacorruption/containerd && sudo /tmp/test -test.v
=== RUN   TestPowerFailureAfterPullImage
    main_test.go:86: Start to run containerd
    main_test.go:90: Wait for ready
    main_test.go:94: Pulling ghcr.io/containerd/alpine:3.14.0
    main_test.go:139: crictl (args: [-r /tmp/TestPowerFailureAfterPullImage4290336617/001/root/run/containerd/containerd.sock pull ghcr.io/containerd/alpine:3.14.0]) output:
         I1118 17:56:47.063177 2514950 util_unix.go:104] "Using this endpoint is deprecated, please consider using full URL format" endpoint="/tmp/TestPowerFailureAfterPullImage4290336617/001/root/run/containerd/containerd.sock" URL="unix:///tmp/TestPowerFailureAfterPullImage4290336617/001/root/run/containerd/containerd.sock"
        Image is up to date for sha256:d4ff818577bc193b309b355b02ebc9220427090057b54a59e73b79bdfe139b83


    main_test.go:97: Power failure
    main_test.go:102: Restarting containerd
    main_test.go:107: Wait for ready
    main_test.go:114: Mounting image ghcr.io/containerd/alpine:3.14.0 on /tmp/TestPowerFailureAfterPullImage4290336617/003
    main_test.go:139: ctr (args: [-a /tmp/TestPowerFailureAfterPullImage4290336617/001/root/run/containerd/containerd.sock -n k8s.io image mount ghcr.io/containerd/alpine:3.14.0 /tmp/TestPowerFailureAfterPullImage4290336617/003]) output:
         sha256:72e830a4dff5f0d5225cdc0a320e85ab1ce06ea5673acfe8d83a7645cbd0e9cf
        /tmp/TestPowerFailureAfterPullImage4290336617/003


    main_test.go:117: Run busybox
    main_test.go:138:
                Error Trace:    github.com/fuweid/go-dmflakey/contrib/datacorruption/containerd/main_test.go:138
                                                        github.com/fuweid/go-dmflakey/contrib/datacorruption/containerd/main_test.go:118
                Error:          Received unexpected error:
                                fork/exec /tmp/TestPowerFailureAfterPullImage4290336617/003/bin/busybox: exec format error
                Test:           TestPowerFailureAfterPullImage
                Messages:       /tmp/TestPowerFailureAfterPullImage4290336617/003/bin/busybox (args: []) output:

--- FAIL: TestPowerFailureAfterPullImage (7.92s)
FAIL
```

### Requirements

The package needs to invoke the following commands to init flakey device:

* [dmsetup.8][dmsetup.8] - low level logical volume management

* [mkfs.8][mkfs.8] - build a Linux filesystem

All of them are supported by most of linux distributions.

[dm-flakey]: <https://docs.kernel.org/admin-guide/device-mapper/dm-flakey.html>
[dmsetup.8]: <https://man7.org/linux/man-pages/man8/dmsetup.8.html>
[mkfs.8]: <https://man7.org/linux/man-pages/man8/mkfs.8.html>
[datacorruption-boltdb]: ./contrib/datacorruption/boltdb
[datacorruption-containerd]: ./contrib/datacorruption/containerd
