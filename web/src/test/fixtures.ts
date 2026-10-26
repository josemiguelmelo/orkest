import type {
  Agent,
  CurrentUser,
  Organization,
  Task,
  TaskIntegrationProvider,
  Workspace,
} from '../shared/api/generated/types.gen';
export const organization: Organization = { id: 'org-1', name: 'Acme engineering', role: 'owner' };
export const workspace: Workspace = {
  id: 'ws-1',
  organizationId: 'org-1',
  name: 'Platform',
  role: 'admin',
};
export const session: CurrentUser = {
  user: { id: 'user-1', username: 'Alex', email: 'alex@example.test' },
  organizations: [organization],
  platformAdmin: false,
};
export const agent: Agent = {
  id: 'agent-1',
  workspaceId: 'ws-1',
  name: 'Implementation agent',
  runtime: 'opencode',
  provider: 'opencode',
  model: 'configured-model',
  description: 'Builds and refines the platform.',
  systemPrompt: 'Work carefully.',
  mcpServerIds: [],
  skillIds: [],
};
export const task: Task = {
  id: 'task-1',
  workspaceId: 'ws-1',
  title: 'Build the management workbench',
  description: 'Create a clear, accessible place to manage project work.',
  sourceProvider: 'manual',
  status: 'todo',
  labels: ['frontend'],
  components: [],
  createdAt: '2026-10-26T10:00:00Z',
  updatedAt: '2026-10-26T11:00:00Z',
  assigneeAgentId: agent.id,
};
export const githubProvider: TaskIntegrationProvider = {
  provider: 'github_cloud',
  configSchema: {
    $schema: 'https://json-schema.org/draft/2020-12/schema',
    type: 'object',
    properties: {
      projectUrl: {
        type: 'string',
        format: 'uri',
        pattern: '^https://github\\.com/(orgs|users)/[^/]+/projects/[1-9][0-9]*$',
        description: 'GitHub Cloud Projects v2 board URL.',
      },
    },
    additionalProperties: false,
    required: ['projectUrl'],
  },
};
