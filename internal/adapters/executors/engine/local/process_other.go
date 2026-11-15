//go:build !unix

package local

import (
	"context"
	"errors"
)

var errProcessMissing = errors.New("process is not running")

func processIdentity(context.Context, int) (string, error) {
	return "", errors.New("local process identity inspection is unavailable on this platform")
}

func signalProcess(int) error {
	return errors.New("local process signaling is unavailable on this platform")
}
