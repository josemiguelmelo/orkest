import { queryOptions, useQueryClient } from '@tanstack/react-query';
import * as sdk from './generated/sdk.gen';
import { command, data } from './client';

export type OrgScope = { userID: string; orgID: string };
export type WorkspaceScope = OrgScope & { workspaceID: string };
export const keys = {
  user: (userID: string) => ['user', userID] as const,
  org: (s: OrgScope) => [...keys.user(s.userID), 'organization', s.orgID] as const,
  workspace: (s: WorkspaceScope) => [...keys.org(s), 'workspace', s.workspaceID] as const,
};
export const path = (s: WorkspaceScope) => ({ orgID: s.orgID, workspaceID: s.workspaceID });
export const meQuery = queryOptions({
  queryKey: ['session'],
  queryFn: ({ signal }) => data(sdk.getCurrentUser({ ...command, signal })),
  retry: false,
  staleTime: 60_000,
});
export const organizationsQuery = (userID: string) =>
  queryOptions({
    queryKey: [...keys.user(userID), 'organizations'],
    queryFn: ({ signal }) => data(sdk.listOrganizations({ ...command, signal })),
  });
export const organizationQuery = (s: OrgScope) =>
  queryOptions({
    queryKey: [...keys.org(s), 'details'],
    queryFn: ({ signal }) =>
      data(sdk.getOrganization({ ...command, path: { orgID: s.orgID }, signal })),
    enabled: !!s.orgID,
  });
export const workspacesQuery = (s: OrgScope) =>
  queryOptions({
    queryKey: [...keys.org(s), 'workspaces'],
    queryFn: ({ signal }) =>
      data(sdk.listWorkspaces({ ...command, path: { orgID: s.orgID }, signal })),
    enabled: !!s.orgID,
  });
export const workspaceQuery = (s: WorkspaceScope) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'details'],
    queryFn: ({ signal }) => data(sdk.getWorkspace({ ...command, path: path(s), signal })),
    enabled: !!s.workspaceID,
  });
export const tasksQuery = (s: WorkspaceScope) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'tasks'],
    queryFn: ({ signal }) => data(sdk.listTasks({ ...command, path: path(s), signal })),
  });
export const taskQuery = (s: WorkspaceScope, taskID: string) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'tasks', taskID],
    queryFn: ({ signal }) =>
      data(sdk.getTask({ ...command, path: { ...path(s), taskID }, signal })),
  });
export const commentsQuery = (s: WorkspaceScope, taskID: string) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'tasks', taskID, 'comments'],
    queryFn: ({ signal }) =>
      data(sdk.listTaskComments({ ...command, path: { ...path(s), taskID }, signal })),
  });
export const taskRunsQuery = (s: WorkspaceScope, taskID: string) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'tasks', taskID, 'runs'],
    queryFn: ({ signal }) =>
      data(sdk.listTaskRuns({ ...command, path: { ...path(s), taskID }, signal })),
    refetchInterval: 2000,
  });
export const taskRunEventsQuery = (s: WorkspaceScope, taskID: string, runID: string) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'tasks', taskID, 'runs', runID, 'events'],
    queryFn: ({ signal }) =>
      data(sdk.listTaskRunEvents({ ...command, path: { ...path(s), taskID, runID }, signal })),
    enabled: !!runID,
    refetchInterval: 1500,
  });
export const agentsQuery = (s: WorkspaceScope) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'agents'],
    queryFn: ({ signal }) => data(sdk.listAgents({ ...command, path: path(s), signal })),
  });
export const componentsQuery = (s: WorkspaceScope) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'components'],
    queryFn: ({ signal }) => data(sdk.listComponents({ ...command, path: path(s), signal })),
  });
export const repositoriesQuery = (s: WorkspaceScope) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'repositories'],
    queryFn: ({ signal }) => data(sdk.listCodeRepositories({ ...command, path: path(s), signal })),
  });
export const codingIntegrationsQuery = (s: WorkspaceScope) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'coding-integrations'],
    queryFn: ({ signal }) => data(sdk.listCodingIntegrations({ ...command, path: path(s), signal })),
  });
export const mcpQuery = (s: WorkspaceScope) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'mcp'],
    queryFn: ({ signal }) => data(sdk.listMcpServers({ ...command, path: path(s), signal })),
  });
export const workspaceEnvironmentQuery = (s: WorkspaceScope) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'environment'],
    queryFn: ({ signal }) =>
      data(sdk.listWorkspaceEnvironment({ ...command, path: path(s), signal })),
  });
export const integrationsQuery = (s: WorkspaceScope) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'integrations'],
    queryFn: ({ signal }) => data(sdk.listTaskIntegrations({ ...command, path: path(s), signal })),
  });
export const providersQuery = (userID: string) =>
  queryOptions({
    queryKey: [...keys.user(userID), 'integration-providers'],
    queryFn: ({ signal }) => data(sdk.listTaskIntegrationProviders({ ...command, signal })),
  });
export const skillsQuery = (s: WorkspaceScope) =>
  queryOptions({
    queryKey: [...keys.workspace(s), 'skills'],
    queryFn: ({ signal }) =>
      s.workspaceID
        ? data(sdk.listWorkspaceSkills({ ...command, path: path(s), signal }))
        : s.orgID
          ? data(sdk.listOrganizationSkills({ ...command, path: { orgID: s.orgID }, signal }))
          : data(sdk.listGlobalSkills({ ...command, signal })),
  });
export const membersQuery = (s: WorkspaceScope, workspace: boolean) =>
  queryOptions({
    queryKey: [...(workspace ? keys.workspace(s) : keys.org(s)), 'members'],
    queryFn: ({ signal }) =>
      workspace
        ? data(sdk.listWorkspaceMembers({ ...command, path: path(s), signal }))
        : data(sdk.listOrganizationMembers({ ...command, path: { orgID: s.orgID }, signal })),
  });
export function useRefresh() {
  const client = useQueryClient();
  return async (s: OrgScope) => {
    await client.invalidateQueries({ queryKey: keys.user(s.userID) });
    await client.invalidateQueries({ queryKey: ['session'] });
  };
}
