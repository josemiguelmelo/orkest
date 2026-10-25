// Package runtime defines the runtimes supported across the platform and agentctl.
package runtime

type Runtime string

const OpenCode Runtime = "opencode"

// Available reports whether the runtime is implemented by both the platform and agentctl.
func (r Runtime) Available() bool {
	return r == OpenCode
}
