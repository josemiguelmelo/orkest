package runtime

import (
	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

// DefaultRuntimePath gives child runtime processes access to standard image
// locations without inheriting the agentctl process environment wholesale.
const DefaultRuntimePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// ResolvedRuntimeSettings intentionally excludes organization, workspace, task,
// run, process, and persistence state from native runtime profiles.
type ResolvedRuntimeSettings struct {
	Agent      runconfig.AgentDefinition
	Model      runconfig.ModelSelection
	MCPServers []runconfig.MCPServer
	Skills     []runconfig.Skill
}

type ProvisioningPaths struct {
	RuntimeRoot  string
	WorkDir      string
	ConfigSubdir string
}

type RuntimeDescriptor struct {
	Runtime                coreruntime.Runtime
	Version                string
	VersionOutputPrefix    string
	VersionOutputSuffix    string
	Executable             string
	Arguments              []string
	VersionArguments       []string
	ReservedEnvPrefixes    []string
	ReservedEnvNames       []string
	ReplayCoverageVerified bool
}

type ProvisioningPlan struct {
	Directories []string
	Files       map[string][]byte
	Environment []string
}

// ProtocolSessionOptions carries native session values through ACP without
// coupling runtime profiles to the ACP SDK types.
type ProtocolSessionOptions struct {
	// Model is the required, runtime-specific configured model identity. The host
	// confirms it after session creation/loading and before any prompt.
	Model      string
	Metadata   map[string]any
	MCPServers []runconfig.MCPServer
}

type RuntimeProfile interface {
	Descriptor() RuntimeDescriptor
	BuildProvisioningPlan(ResolvedRuntimeSettings, ProvisioningPaths) (ProvisioningPlan, error)
	NativeSessionMode(ResolvedRuntimeSettings) string
	ProtocolSessionOptions(ResolvedRuntimeSettings, ProvisioningPaths) ProtocolSessionOptions
}
