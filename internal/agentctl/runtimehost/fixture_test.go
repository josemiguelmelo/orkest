package runtimehost_test

import (
	"os"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/opencode"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/opencode/opencodetest"
)

func TestMain(m *testing.M) {
	opencodetest.Run(opencode.Version)
	os.Exit(m.Run())
}
