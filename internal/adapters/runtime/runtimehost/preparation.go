package runtimehost

import (
	"context"
	"slices"

	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// RuntimePreparationInput contains the provider-neutral inputs used to prepare
// a runtime's environment before provisioning and launch.
type RuntimePreparationInput struct {
	Settings           agentruntime.ResolvedRuntimeSettings
	Paths              agentruntime.ProvisioningPaths
	Plan               agentruntime.ProvisioningPlan
	GrantedEnvironment []string
	Restoring          bool
}

// RuntimePreparationResult carries provider-specific plan changes and secret
// values that the host must redact from runtime activity.
type RuntimePreparationResult struct {
	Plan               agentruntime.ProvisioningPlan
	GrantedEnvironment []string
	SecretValues       []string
}

// RuntimePreparationStrategy handles host-side provider preparation that does
// not belong in a declarative RuntimeProfile provisioning plan.
type RuntimePreparationStrategy interface {
	Prepare(context.Context, RuntimePreparationInput) (RuntimePreparationResult, error)
}

// NoopRuntimePreparationStrategy is used by runtimes without extra host-side
// preparation requirements.
type NoopRuntimePreparationStrategy struct{}

func (NoopRuntimePreparationStrategy) Prepare(_ context.Context, input RuntimePreparationInput) (RuntimePreparationResult, error) {
	return RuntimePreparationResult{
		Plan:               input.Plan,
		GrantedEnvironment: slices.Clone(input.GrantedEnvironment),
	}, nil
}
