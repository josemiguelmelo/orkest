package main

import "os"

type kubernetesServerConfiguration struct {
	Namespace, InstallationID, DataPVC, DataSubpath            string
	ServiceAccount, UID, GID, StartupTimeout, TerminationGrace string
	CodexAuthSecret, CodexAuthSecretKey, ImagePullSecrets      string
	CPURequest, MemoryRequest, CPULimit, MemoryLimit           string
	PlatformServiceName, PlatformServicePort                   string
}

// serverConfiguration snapshots composition settings once before startup I/O.
type serverConfiguration struct {
	DataDir                      string
	HTTPAddr                     string
	IntegrationEncryptionKey     string
	AgentctlOpencodeImage        string
	AgentctlClaudeCodeImage      string
	AgentctlCodexImage           string
	AgentctlGitHubCopilotImage   string
	AgentctlKiroImage            string
	ExecutionImageRepository     string
	Executor                     string
	AgentctlNetwork              string
	CodexAuthFile                string
	AgentctlBinary               string
	AgentProviderEnv             string
	AgentMCPURL                  string
	AgentMCPTokenKey             string
	DockerRegistryHost           string
	BuildKitRegistryHost         string
	ImageBuilder                 string
	BuildctlBinary               string
	BuildKitAddr                 string
	BuildKitTLSCA                string
	BuildKitTLSCert              string
	BuildKitTLSKey               string
	BuildKitDaemonless           string
	ExecutionProfileBuildWorkers string
	Kubernetes                   kubernetesServerConfiguration
}

func loadServerConfiguration() serverConfiguration {
	return serverConfiguration{
		DataDir:                      os.Getenv("ORKEST_DATA_DIR"),
		HTTPAddr:                     os.Getenv("HTTP_ADDR"),
		IntegrationEncryptionKey:     os.Getenv("ORKEST_INTEGRATION_ENCRYPTION_KEY"),
		AgentctlOpencodeImage:        os.Getenv("AGENTCTL_OPENCODE_IMAGE"),
		AgentctlClaudeCodeImage:      os.Getenv("AGENTCTL_CLAUDE_CODE_IMAGE"),
		AgentctlCodexImage:           os.Getenv("AGENTCTL_CODEX_IMAGE"),
		AgentctlGitHubCopilotImage:   os.Getenv("AGENTCTL_GITHUB_COPILOT_IMAGE"),
		AgentctlKiroImage:            os.Getenv("AGENTCTL_KIRO_IMAGE"),
		ExecutionImageRepository:     os.Getenv("ORKEST_EXECUTION_IMAGE_REPOSITORY"),
		Executor:                     os.Getenv("ORKEST_EXECUTOR"),
		AgentctlNetwork:              os.Getenv("ORKEST_AGENTCTL_NETWORK"),
		CodexAuthFile:                os.Getenv("ORKEST_CODEX_AUTH_FILE"),
		AgentctlBinary:               os.Getenv("ORKEST_AGENTCTL_BINARY"),
		AgentProviderEnv:             os.Getenv("ORKEST_AGENT_PROVIDER_ENV"),
		AgentMCPURL:                  os.Getenv("ORKEST_AGENT_MCP_URL"),
		AgentMCPTokenKey:             os.Getenv("ORKEST_AGENT_MCP_TOKEN_KEY"),
		DockerRegistryHost:           os.Getenv("ORKEST_DOCKER_REGISTRY_HOST"),
		BuildKitRegistryHost:         os.Getenv("ORKEST_BUILDKIT_REGISTRY_HOST"),
		ImageBuilder:                 os.Getenv("ORKEST_IMAGE_BUILDER"),
		BuildctlBinary:               os.Getenv("ORKEST_BUILDCTL_BINARY"),
		BuildKitAddr:                 os.Getenv("ORKEST_BUILDKIT_ADDR"),
		BuildKitTLSCA:                os.Getenv("ORKEST_BUILDKIT_TLS_CA"),
		BuildKitTLSCert:              os.Getenv("ORKEST_BUILDKIT_TLS_CERT"),
		BuildKitTLSKey:               os.Getenv("ORKEST_BUILDKIT_TLS_KEY"),
		BuildKitDaemonless:           os.Getenv("ORKEST_BUILDKIT_DAEMONLESS"),
		ExecutionProfileBuildWorkers: os.Getenv("ORKEST_EXECUTION_PROFILE_BUILD_WORKERS"),
		Kubernetes: kubernetesServerConfiguration{
			Namespace:           os.Getenv("ORKEST_KUBERNETES_NAMESPACE"),
			InstallationID:      os.Getenv("ORKEST_KUBERNETES_INSTALLATION_ID"),
			DataPVC:             os.Getenv("ORKEST_KUBERNETES_DATA_PVC"),
			DataSubpath:         os.Getenv("ORKEST_KUBERNETES_DATA_SUBPATH"),
			ServiceAccount:      os.Getenv("ORKEST_KUBERNETES_SERVICE_ACCOUNT"),
			UID:                 os.Getenv("ORKEST_KUBERNETES_UID"),
			GID:                 os.Getenv("ORKEST_KUBERNETES_GID"),
			StartupTimeout:      os.Getenv("ORKEST_KUBERNETES_STARTUP_TIMEOUT"),
			TerminationGrace:    os.Getenv("ORKEST_KUBERNETES_TERMINATION_GRACE_SECONDS"),
			CodexAuthSecret:     os.Getenv("ORKEST_KUBERNETES_CODEX_AUTH_SECRET"),
			CodexAuthSecretKey:  os.Getenv("ORKEST_KUBERNETES_CODEX_AUTH_SECRET_KEY"),
			ImagePullSecrets:    os.Getenv("ORKEST_KUBERNETES_IMAGE_PULL_SECRETS"),
			CPURequest:          os.Getenv("ORKEST_KUBERNETES_CPU_REQUEST"),
			MemoryRequest:       os.Getenv("ORKEST_KUBERNETES_MEMORY_REQUEST"),
			CPULimit:            os.Getenv("ORKEST_KUBERNETES_CPU_LIMIT"),
			MemoryLimit:         os.Getenv("ORKEST_KUBERNETES_MEMORY_LIMIT"),
			PlatformServiceName: os.Getenv("ORKEST_KUBERNETES_PLATFORM_SERVICE_NAME"),
			PlatformServicePort: os.Getenv("ORKEST_KUBERNETES_PLATFORM_SERVICE_PORT"),
		},
	}
}
