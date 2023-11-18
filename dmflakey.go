//go:build linux

package dmflakey

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type featCfg struct {
	// SyncFS attempts to synchronize filesystem before inject failure.
	syncFS bool
	// interval is used to determine how long the failure lasts.
	interval time.Duration
}

var defaultFeatCfg = featCfg{interval: defaultInterval}

// FeatOpt is used to configure failure feature.
type FeatOpt func(*featCfg)

// WithIntervalFeatOpt updates the up time for the feature.
func WithIntervalFeatOpt(interval time.Duration) FeatOpt {
	return func(cfg *featCfg) {
		cfg.interval = interval
	}
}

// WithSyncFSFeatOpt is to determine if the caller wants to synchronize
// filesystem before inject failure.
func WithSyncFSFeatOpt(syncFS bool) FeatOpt {
	return func(cfg *featCfg) {
		cfg.syncFS = syncFS
	}
}

// Flakey is to inject failure into device.
type Flakey interface {
	// DevicePath returns the flakey device path.
	DevicePath() string

	// AllowWrites allows write I/O.
	AllowWrites(opts ...FeatOpt) error

	// DropWrites drops all write I/O silently.
	DropWrites(opts ...FeatOpt) error

	// ErrorWrites drops all write I/O and returns error.
	ErrorWrites(opts ...FeatOpt) error

	// ErrorReads makes all read I/O is failed with an error signalled.
	ErrorReads(opts ...FeatOpt) error

	// CorruptBIOByte corruptes one byte in write bio.
	CorruptBIOByte(nth int, read bool, value uint8, flags int, opts ...FeatOpt) error

	// RandomReadCorrupt replaces random byte in a read bio with a random value.
	RandomReadCorrupt(probability int, opts ...FeatOpt) error

	// RandomWriteCorrupt replaces random byte in a write bio with a random value.
	RandomWriteCorrupt(probability int, opts ...FeatOpt) error

	// Teardown releases the flakey device.
	Teardown() error
}

// FSType represents the filesystem name.
type FSType string

// Supported filesystems.
const (
	FSTypeEXT4 FSType = "ext4"
	FSTypeXFS  FSType = "xfs"
)

// Default values.
var (
	defaultImgSize  int64 = 1024 * 1024 * 1024 * 10 // 10 GiB
	defaultInterval       = 2 * time.Minute
)

// InitFlakey creates an filesystem on a loopback device and returns Flakey on it.
//
// The device-mapper device will be /dev/mapper/$flakeyDevice. And the filesystem
// image will be created at $dataStorePath/$flakeyDevice.img. By default, the
// device is available for 2 minutes and size is 10 GiB.
func InitFlakey(flakeyDevice, dataStorePath string, fsType FSType) (_ Flakey, retErr error) {
	imgPath := filepath.Join(dataStorePath, fmt.Sprintf("%s.img", flakeyDevice))
	if err := createEmptyFSImage(imgPath, fsType); err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			os.RemoveAll(imgPath)
		}
	}()

	loopDevice, err := attachToLoopDevice(imgPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			detachLoopDevice(loopDevice)
		}
	}()

	imgSize, err := getBlkSize(loopDevice)
	if err != nil {
		return nil, err
	}

	if err := newFlakeyDevice(flakeyDevice, loopDevice, defaultInterval); err != nil {
		return nil, err
	}

	return &flakey{
		fsType:  fsType,
		imgPath: imgPath,
		imgSize: imgSize,

		loopDevice:   loopDevice,
		flakeyDevice: flakeyDevice,
	}, nil
}

type flakey struct {
	fsType  FSType
	imgPath string
	imgSize int64

	loopDevice   string
	flakeyDevice string
}

// DevicePath returns the flakey device path.
func (f *flakey) DevicePath() string {
	return fmt.Sprintf("/dev/mapper/%s", f.flakeyDevice)
}

// AllowWrites allows write I/O.
func (f *flakey) AllowWrites(opts ...FeatOpt) error {
	var o = defaultFeatCfg
	for _, opt := range opts {
		opt(&o)
	}

	table := fmt.Sprintf("0 %d flakey %s 0 %d 0",
		f.imgSize, f.loopDevice, int(o.interval.Seconds()))

	return reloadFlakeyDevice(f.flakeyDevice, o.syncFS, table)
}

// DropWrites drops all write I/O silently.
func (f *flakey) DropWrites(opts ...FeatOpt) error {
	var o = defaultFeatCfg
	for _, opt := range opts {
		opt(&o)
	}

	table := fmt.Sprintf("0 %d flakey %s 0 0 %d 1 drop_writes",
		f.imgSize, f.loopDevice, int(o.interval.Seconds()))

	return reloadFlakeyDevice(f.flakeyDevice, o.syncFS, table)
}

// ErrorWrites drops all write I/O and returns error.
func (f *flakey) ErrorWrites(opts ...FeatOpt) error {
	var o = defaultFeatCfg
	for _, opt := range opts {
		opt(&o)
	}

	table := fmt.Sprintf("0 %d flakey %s 0 0 %d 1 error_writes",
		f.imgSize, f.loopDevice, int(o.interval.Seconds()))

	return reloadFlakeyDevice(f.flakeyDevice, o.syncFS, table)
}

// ErrorReads makes all read I/O is failed with an error signalled.
func (f *flakey) ErrorReads(opts ...FeatOpt) error {
	return errors.New("not implemented")
}

// CorruptBIOByte corruptes one byte in write bio.
func (f *flakey) CorruptBIOByte(nth int, read bool, value uint8, flags int, opts ...FeatOpt) error {
	return errors.New("not implemented")
}

// RandomReadCorrupt replaces random byte in a read bio with a random value.
func (f *flakey) RandomReadCorrupt(probability int, opts ...FeatOpt) error {
	return errors.New("not implemented")
}

// RandomWriteCorrupt replaces random byte in a write bio with a random value.
func (f *flakey) RandomWriteCorrupt(probability int, opts ...FeatOpt) error {
	return errors.New("not implemented")
}

// Teardown releases the flakey device.
func (f *flakey) Teardown() error {
	if err := deleteFlakeyDevice(f.flakeyDevice); err != nil {
		if !strings.Contains(err.Error(), "No such device or address") {
			return err
		}
	}
	if err := detachLoopDevice(f.loopDevice); err != nil {
		if !errors.Is(err, unix.ENXIO) {
			return err
		}
	}
	return os.RemoveAll(f.imgPath)
}

// createEmptyFSImage creates empty filesystem on dataStorePath folder with
// default size - 10 GiB.
func createEmptyFSImage(imgPath string, fsType FSType) error {
	if err := validateFSType(fsType); err != nil {
		return err
	}

	mkfs, err := exec.LookPath(fmt.Sprintf("mkfs.%s", fsType))
	if err != nil {
		return fmt.Errorf("failed to ensure mkfs.%s: %w", fsType, err)
	}

	if _, err := os.Stat(imgPath); err == nil {
		return fmt.Errorf("failed to create image because %s already exists", imgPath)
	}

	f, err := os.Create(imgPath)
	if err != nil {
		return fmt.Errorf("failed to create image %s: %w", imgPath, err)
	}

	if err = func() error {
		defer f.Close()

		return f.Truncate(defaultImgSize)
	}(); err != nil {
		return fmt.Errorf("failed to truncate image %s with %v bytes: %w",
			imgPath, defaultImgSize, err)
	}

	output, err := exec.Command(mkfs, imgPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to mkfs.%s on %s (out: %s): %w",
			fsType, imgPath, string(output), err)
	}
	return nil
}

// validateFSType validates the fs type input.
func validateFSType(fsType FSType) error {
	switch fsType {
	case FSTypeEXT4, FSTypeXFS:
		return nil
	default:
		return fmt.Errorf("unsupported filesystem %s", fsType)
	}
}
