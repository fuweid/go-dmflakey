//go:build linux

package main

import (
	"os"
	"testing"

	"github.com/fuweid/go-dmflakey/contrib/testutils"
)

func TestMain(m *testing.M) {
	testutils.RequiresRoot()
	testutils.RequiresCommands("bbolt")
	os.Exit(m.Run())
}
