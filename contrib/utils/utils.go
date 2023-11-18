//go:build linux

package utils

import "github.com/fuweid/go-dmflakey"

// SimulatePowerFailure is used to simulate power failure with drop all the writes.
func SimulatePowerFailure(flakey dmflakey.Flakey, root string) error {
	if err := flakey.DropWrites(); err != nil {
		return err
	}

	if err := Unmount(root); err != nil {
		return err
	}

	if err := flakey.AllowWrites(); err != nil {
		return err
	}
	return Mount(root, flakey.DevicePath(), "")
}
