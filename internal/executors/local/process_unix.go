//go:build unix

package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

var errProcessMissing = errors.New("process is not running")

func processIdentity(ctx context.Context, pid int) (string, error) {
	if pid < 1 {
		return "", errors.New("invalid local agentctl PID")
	}
	output, err := exec.CommandContext(ctx, "ps", "ww", "-o", "lstart=", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(strings.TrimSpace(string(exit.Stderr))) == 0 {
			return "", errProcessMissing
		}
		return "", fmt.Errorf("inspect local agentctl process: %w", err)
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", errProcessMissing
	}
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:]), nil
}

func osProcessSignal(pid int) error {
	process, err := findProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGTERM)
}

func findProcess(pid int) (*os.Process, error) { return os.FindProcess(pid) }
