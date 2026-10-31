# Agent runtime profiles

Agent runtime profiles are workspace scoped named presets for a runtime, model
provider, and model identifier. They simplify agent creation while preserving
direct configuration for custom runtime and gateway setups.

An agent may link one profile. The agent keeps a copy of the selected settings
for execution and exposes the profile ID for future edits. Creating or editing
an agent with a profile resolves its current settings. Updating a profile
changes the runtime/provider/model fields of all linked agents in the same
database transaction as the profile update. Existing task runs already have
their effective launch configuration and are not rewritten. A profile update
that conflicts with an agent's selected execution image profile is rejected.

Deleting a profile clears agent links but leaves their current runtime,
provider, and model values intact. Runtime profiles and execution profiles
(toolchain images) are separate resources and can be selected independently
when their runtimes are compatible.

The authenticated workspace API is rooted at
`/api/organizations/{orgID}/workspaces/{workspaceID}/agent-runtime-profiles`.
Workspace viewers have read access and cannot create, update, or delete
profiles.
