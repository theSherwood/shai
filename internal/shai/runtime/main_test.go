package shai

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Force verbose mode during tests so setup logs remain visible.
	_ = os.Setenv("SHAI_FORCE_VERBOSE", "1")
	os.Exit(m.Run())
}
