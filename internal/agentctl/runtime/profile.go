package runtime

import (
	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
)

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

type RuntimeProfile interface {
	Descriptor() RuntimeDescriptor
	BuildProvisioningPlan(ResolvedRuntimeSettings, ProvisioningPaths) (ProvisioningPlan, error)
	NativeSessionMode(ResolvedRuntimeSettings) string
}
