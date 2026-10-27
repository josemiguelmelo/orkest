// Package runtime defines the runtimes supported across the platform and agentctl.
package runtime

import (
	_ "embed"
	"encoding/json"
)

//go:embed versions.json
var catalog []byte

// Supported returns a copy of the statically enabled runtime/version inventory.
func Supported() map[Runtime]string {
	var versions map[Runtime]string
	if err := json.Unmarshal(catalog, &versions); err != nil {
		panic(err)
	}
	return versions
}

type Runtime string

const OpenCode Runtime = "opencode"

// Available reports whether the runtime is implemented by both the platform and agentctl.
func (r Runtime) Available() bool {
	_, ok := Supported()[r]
	return ok
}
