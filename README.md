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

### Requirements

The package needs to invoke the following commands to init flakey device:

* [dmsetup.8][dmsetup.8] - low level logical volume management

* [mkfs.8][mkfs.8] - build a Linux filesystem

All of them are supported by most of linux distributions.

[dm-flakey]: <https://docs.kernel.org/admin-guide/device-mapper/dm-flakey.html>
[dmsetup.8]: <https://man7.org/linux/man-pages/man8/dmsetup.8.html>
[mkfs.8]: <https://man7.org/linux/man-pages/man8/mkfs.8.html>
[datacorruption-boltdb]: ./contrib/datacorruption/boltdb
