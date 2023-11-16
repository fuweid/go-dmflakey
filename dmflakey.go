package dmflakey

import (
	"context"
	"errors"
	"time"
)

var defaultInterval = 2 * time.Minute

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
	// DevicePath returns the loopback device path.
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
	RandomReadCorrupt(probability int) error

	// RandomWriteCorrupt replaces random byte in a write bio with a random value.
	RandomWriteCorrupt(probability int) error

	// Teardown releases the loopback device.
	Teardown() error
}

// FSType represents the filesystem name.
type FSType string

// Supported filesystems.
const (
	FSTypeEXT4 FSType = "ext4"
	FSTypeXFS  FSType = "xfs"
)

// InitFlakey creates an filesystem on a loopback device and returns Flakey
// on it. By default, the device is available for 2 minutes.
func InitFlakey(ctx context.Context, fsType FSType) (Flakey, error) {
	return nil, errors.New("not implemented")
}
