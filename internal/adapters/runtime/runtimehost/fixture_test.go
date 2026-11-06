package runtimehost_test

import (
	"os"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/opencode"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/opencode/opencodetest"
)

func TestMain(m *testing.M) {
	opencodetest.Run(opencode.Version)
	os.Exit(m.Run())
}
