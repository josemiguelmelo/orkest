// Package local implements the native-process infrastructure engine.
package local

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
)

type Config struct {
	DataRoot     string
	LogForwarder engine.LogForwarder
}

type Engine struct {
	dataRoot     string
	logForwarder engine.LogForwarder
	installation string
	manifests    *engine.ManifestStore
	mu           sync.Mutex
	processes    map[string]*os.Process
}

var _ engine.InfrastructureEngine = (*Engine)(nil)

func New(config Config) (*Engine, error) {
	root := strings.TrimSpace(config.DataRoot)
	if root == "" {
		return nil, errors.New("local engine data root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local engine data root: %w", err)
	}
	hash := sha256.Sum256([]byte(root))
	manifests, err := engine.NewManifestStore(filepath.Join(root, "infrastructure-operations"))
	if err != nil {
		return nil, err
	}
	return &Engine{dataRoot: root, logForwarder: config.LogForwarder, installation: hex.EncodeToString(hash[:]), manifests: manifests, processes: make(map[string]*os.Process)}, nil
}

func (e *Engine) Capabilities() engine.Capabilities {
	return engine.Capabilities{
		Modes:                []engine.ExecutionMode{engine.ExecutionModeNative},
		DurableWork:          true,
		DurableState:         true,
		PlatformStorageSetup: true,
	}
}

func (*Engine) Close() error { return nil }

func parseIdentity(value string) (int, string, error) {
	pidValue, token, ok := strings.Cut(strings.TrimSpace(value), ":")
	if !ok || token == "" {
		return 0, "", errors.New("invalid local engine process identity")
	}
	pid, err := strconv.Atoi(pidValue)
	if err != nil || pid < 1 {
		return 0, "", errors.New("invalid local engine process identity")
	}
	return pid, token, nil
}
