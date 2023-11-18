## go-dmflakey

Go bindings for the [dm-flakey][dm-flakey] device.

### Key Features

* `AllowWrites`: The device is available for `<up interval>` seconds.

* `DropWrites`: All write I/O is silently ignored for `<down interval>` seconds. Read I/O is handled correctly.

* `ErrorWrites`: All write I/O is failed with an error signalled for `<down interval>` seconds. Read I/O is handled correctly.

Both `<up>` and `<down>` interval can be configured by `WithIntervalFeatOpt(interval)`.

TODO: `ErrorReads`, `CorruptBIOByte`, `RandomReadCorrupt`, `RandomWriteCorrupt`.

### Example

Simulate power failure and cause data loss

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

### Requirements

The package needs to invoke the following commands to init flakey device:

* [dmsetup.8][dmsetup.8] - low level logical volume management

* [mkfs.8][mkfs.8] - mke2fs - create an ext2/ext3/ext4 filesystem

All of them are supported by most of linux distributions.

[dm-flakey]: <https://docs.kernel.org/admin-guide/device-mapper/dm-flakey.html>
[dmsetup]: <https://man7.org/linux/man-pages/man8/dmsetup.8.html>
[mkfs.8]: <https://man7.org/linux/man-pages/man8/mkfs.8.html>
