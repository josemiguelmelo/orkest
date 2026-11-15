// Package agentctl contains agent workload planning and execution policy.
package agentctl

import "github.com/josemiguelmelo/orkest/internal/adapters/executors/checkout"

// WorkspaceConfig contains the shared repository preparation dependency.
type WorkspaceConfig struct {
	Repositories checkout.Preparer
}
