import { lazy, Suspense } from 'react';
import { createRootRoute, createRoute, createRouter, Outlet } from '@tanstack/react-router';
import { z } from 'zod';
import { Shell } from './shell';
import { ErrorState, Loading } from '../shared/ui/common';

const Home = lazy(() =>
  import('../features/settings/organizations').then((m) => ({ default: m.OrganizationsPage })),
);
const Organization = lazy(() =>
  import('../features/settings/organizations').then((m) => ({ default: m.OrganizationPage })),
);
const Tasks = lazy(() => import('../features/tasks/tasks').then((m) => ({ default: m.TasksPage })));
const Task = lazy(() => import('../features/tasks/tasks').then((m) => ({ default: m.TaskPage })));
const Agents = lazy(() =>
  import('../features/agents/agents').then((m) => ({ default: m.AgentsPage })),
);
const Skills = lazy(() =>
  import('../features/skills/skills').then((m) => ({ default: m.SkillsPage })),
);
const Components = lazy(() =>
  import('../features/settings/components').then((m) => ({ default: m.ComponentsPage })),
);
const Repositories = lazy(() =>
  import('../features/settings/repositories').then((m) => ({ default: m.RepositoriesPage })),
);
const Mcp = lazy(() => import('../features/settings/mcp').then((m) => ({ default: m.McpPage })));
const Integrations = lazy(() =>
  import('../features/integrations/integrations').then((m) => ({ default: m.IntegrationsPage })),
);
const WorkspaceSettings = lazy(() =>
  import('../features/settings/organizations').then((m) => ({ default: m.WorkspaceSettingsPage })),
);
const root = createRootRoute({
  component: Shell,
  notFoundComponent: () => (
    <div className="page">
      <h1>Page not found</h1>
      <p className="muted">Use the navigation to choose a workspace or resource.</p>
    </div>
  ),
  errorComponent: ({ error, reset }) => (
    <div className="page">
      <ErrorState error={error} retry={reset} />
    </div>
  ),
});
const loading = (Component: typeof Home) => () => (
  <Suspense fallback={<Loading />}>
    <Component />
  </Suspense>
);
const home = createRoute({ getParentRoute: () => root, path: '/', component: loading(Home) });
const globalSkills = createRoute({
  getParentRoute: () => root,
  path: 'skills',
  component: loading(Skills),
});
const organization = createRoute({
  getParentRoute: () => root,
  path: 'organizations/$orgId',
  component: Outlet,
});
const organizationIndex = createRoute({
  getParentRoute: () => organization,
  path: '/',
  component: loading(Organization),
});
const organizationSkills = createRoute({
  getParentRoute: () => organization,
  path: 'skills',
  component: loading(Skills),
});
const workspace = createRoute({
  getParentRoute: () => organization,
  path: 'workspaces/$workspaceId',
  component: Outlet,
});
export const taskSearchSchema = z.object({
  view: z.enum(['board', 'list']).catch('board'),
  q: z.string().catch(''),
  status: z.enum(['all', 'todo', 'in_progress', 'in_review', 'done']).catch('all'),
  agent: z.string().catch(''),
  sort: z.enum(['updated', 'title']).catch('updated'),
});
export const tasksRoute = createRoute({
  getParentRoute: () => workspace,
  path: 'tasks',
  validateSearch: taskSearchSchema,
  component: loading(Tasks),
});
export const taskRoute = createRoute({
  getParentRoute: () => workspace,
  path: 'tasks/$taskId',
  component: loading(Task),
});
const workspaceIndex = createRoute({
  getParentRoute: () => workspace,
  path: '/',
  component: loading(Tasks),
  validateSearch: taskSearchSchema,
});
const agents = createRoute({
  getParentRoute: () => workspace,
  path: 'agents',
  component: loading(Agents),
});
const skills = createRoute({
  getParentRoute: () => workspace,
  path: 'skills',
  component: loading(Skills),
});
const components = createRoute({
  getParentRoute: () => workspace,
  path: 'components',
  component: loading(Components),
});
const repositories = createRoute({
  getParentRoute: () => workspace,
  path: 'repositories',
  component: loading(Repositories),
});
const mcp = createRoute({
  getParentRoute: () => workspace,
  path: 'mcp-servers',
  component: loading(Mcp),
});
const integrations = createRoute({
  getParentRoute: () => workspace,
  path: 'integrations',
  component: loading(Integrations),
});
const settings = createRoute({
  getParentRoute: () => workspace,
  path: 'settings',
  component: loading(WorkspaceSettings),
});
const tree = root.addChildren([
  home,
  globalSkills,
  organization.addChildren([
    organizationIndex,
    organizationSkills,
    workspace.addChildren([
      workspaceIndex,
      tasksRoute,
      taskRoute,
      agents,
      skills,
      components,
      repositories,
      mcp,
      integrations,
      settings,
    ]),
  ]),
]);
export const router = createRouter({ routeTree: tree, defaultPreload: 'intent' });
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
