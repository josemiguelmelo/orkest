import type { JSX } from 'react';
import { useEffect, useState } from 'react';
import { Link, Outlet, useNavigate, useParams } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Bot,
  ChevronRight,
  CircleHelp,
  FolderGit2,
  Layers,
  LogOut,
  Menu,
  Network,
  NotebookText,
  Plug,
  Settings,
  Shield,
  Workflow,
  X,
} from 'lucide-react';
import type { CurrentUser } from '../shared/api/generated/types.gen';
import { AppContext } from './context';
import { capabilities } from './permissions';
import { ApiError, command, data } from '../shared/api/client';
import * as sdk from '../shared/api/generated/sdk.gen';
import {
  meQuery,
  organizationsQuery,
  organizationQuery,
  workspaceQuery,
  workspacesQuery,
  type WorkspaceScope,
} from '../shared/api/queries';
import { Button } from '../shared/ui/button';
import { Badge, ErrorState, Loading } from '../shared/ui/common';

export function Shell(): JSX.Element {
  const client = useQueryClient();
  const me = useQuery(meQuery);
  const [expired, setExpired] = useState(false);
  useEffect(() => {
    const expire = () => {
      setExpired(true);
      client.clear();
    };
    window.addEventListener('orkest:session-expired', expire);
    return () => window.removeEventListener('orkest:session-expired', expire);
  }, [client]);
  if (expired || (me.error instanceof ApiError && me.error.status === 401)) return <Login />;
  if (me.isPending) return <Loading />;
  if (me.isError)
    return (
      <div className="login">
        <ErrorState
          error={me.error}
          retry={() => {
            void me.refetch();
          }}
        />
      </div>
    );
  return <AuthenticatedShell session={me.data} />;
}
function Login() {
  return (
    <main className="login">
      <section className="login-card">
        <div className="brand">
          <span className="brand-mark">o</span>orkest
        </div>
        <h1>Your workspace for agent work.</h1>
        <p className="muted">
          Organize tasks, configure agents, and connect the tools your team uses.
        </p>
        <a className="button button-primary" href="/auth/login">
          Sign in with Keycloak <ChevronRight size={16} />
        </a>
        <p className="muted spacer">Authentication is managed by your organization.</p>
      </section>
    </main>
  );
}
function AuthenticatedShell({ session }: { session: CurrentUser }) {
  return <ShellContent session={session} />;
}
function ShellContent({ session }: { session: CurrentUser }) {
  const params = useParams({ strict: false });
  const navigate = useNavigate();
  const client = useQueryClient();
  const scope: WorkspaceScope = {
    userID: session.user.id,
    orgID: params.orgId ?? '',
    workspaceID: params.workspaceId ?? '',
  };
  const organizations = useQuery(organizationsQuery(scope.userID));
  const organization = useQuery(organizationQuery(scope));
  const workspaces = useQuery(workspacesQuery(scope));
  const workspace = useQuery(workspaceQuery(scope));
  const [mobile, setMobile] = useState(false);
  const [logoutError, setLogoutError] = useState<unknown>();
  const permissions = capabilities(
    organization.data?.role,
    workspace.data?.role,
    session.platformAdmin,
  );
  useEffect(
    () => () => {
      void client.cancelQueries({ queryKey: ['user', scope.userID, 'organization', scope.orgID] });
    },
    [client, scope.userID, scope.orgID, scope.workspaceID],
  );
  async function logout() {
    try {
      await data(sdk.logout(command));
      client.clear();
      window.location.assign('/');
    } catch (error) {
      setLogoutError(error);
    }
  }
  const scoped = !!scope.orgID && !!scope.workspaceID;
  const workspaceParams = { orgId: scope.orgID, workspaceId: scope.workspaceID };
  const blocked = (scope.orgID && organization.isError) || (scope.workspaceID && workspace.isError);
  const waiting =
    (scope.orgID && organization.isPending) || (scope.workspaceID && workspace.isPending);
  return (
    <AppContext.Provider
      value={{
        session,
        scope,
        organization: organization.data,
        workspace: workspace.data,
        permissions,
      }}
    >
      <a href="#main-content" className="skip-link">
        Skip to content
      </a>
      <div className="app-shell">
        <aside className="sidebar" data-open={mobile} aria-label="Main navigation">
          <div className="split">
            <Link to="/" className="brand" onClick={() => setMobile(false)}>
              <span className="brand-mark">o</span>orkest
            </Link>
            <Button
              variant="ghost"
              size="icon"
              className="mobile-toggle"
              aria-label="Close navigation"
              onClick={() => setMobile(false)}
            >
              <X size={18} />
            </Button>
          </div>
          <div className="scope-switch">
            <label htmlFor="organization-switch">Organization</label>
            <select
              id="organization-switch"
              value={scope.orgID}
              onChange={(e) => {
                setMobile(false);
                void navigate(
                  e.target.value
                    ? { to: '/organizations/$orgId', params: { orgId: e.target.value } }
                    : { to: '/' },
                );
              }}
            >
              <option value="">Choose organization</option>
              {organizations.data?.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
            {scope.orgID && (
              <>
                <label htmlFor="workspace-switch">Workspace</label>
                <select
                  id="workspace-switch"
                  value={scope.workspaceID}
                  onChange={(e) => {
                    setMobile(false);
                    void navigate(
                      e.target.value
                        ? {
                            to: '/organizations/$orgId/workspaces/$workspaceId/tasks',
                            params: { orgId: scope.orgID, workspaceId: e.target.value },
                            search: {
                              view: 'board',
                              q: '',
                              status: 'all',
                              agent: '',
                              sort: 'updated',
                            },
                          }
                        : { to: '/organizations/$orgId', params: { orgId: scope.orgID } },
                    );
                  }}
                >
                  <option value="">Choose workspace</option>
                  {workspaces.data?.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name}
                    </option>
                  ))}
                </select>
              </>
            )}
          </div>
          {scoped && (
            <nav className="nav-group" onClick={() => setMobile(false)}>
              <p className="nav-label">Workspace</p>
              <Link
                className="nav-link"
                to="/organizations/$orgId/workspaces/$workspaceId/tasks"
                params={workspaceParams}
                search={{ view: 'board', q: '', status: 'all', agent: '', sort: 'updated' }}
              >
                <Workflow size={17} />
                Tasks
              </Link>
              <Link
                className="nav-link"
                to="/organizations/$orgId/workspaces/$workspaceId/agents"
                params={workspaceParams}
              >
                <Bot size={17} />
                Agents
              </Link>
              <Link
                className="nav-link"
                to="/organizations/$orgId/workspaces/$workspaceId/skills"
                params={workspaceParams}
              >
                <NotebookText size={17} />
                Skills
              </Link>
              <p className="nav-label spacer">Configuration</p>
              <Link
                className="nav-link"
                to="/organizations/$orgId/workspaces/$workspaceId/repositories"
                params={workspaceParams}
              >
                <FolderGit2 size={17} />
                Repositories
              </Link>
              <Link
                className="nav-link"
                to="/organizations/$orgId/workspaces/$workspaceId/integrations"
                params={workspaceParams}
              >
                <Plug size={17} />
                Integrations
              </Link>
              <Link
                className="nav-link"
                to="/organizations/$orgId/workspaces/$workspaceId/mcp-servers"
                params={workspaceParams}
              >
                <Network size={17} />
                MCP servers
              </Link>
              <Link
                className="nav-link"
                to="/organizations/$orgId/workspaces/$workspaceId/settings"
                params={workspaceParams}
              >
                <Settings size={17} />
                Settings & members
              </Link>
            </nav>
          )}
          <nav className="nav-group" onClick={() => setMobile(false)}>
            <p className="nav-label">Organization</p>
            <Link className="nav-link" to="/" activeOptions={{ exact: true }}>
              <Layers size={17} />
              All organizations
            </Link>
            {scope.orgID && (
              <>
                <Link
                  className="nav-link"
                  to="/organizations/$orgId"
                  params={{ orgId: scope.orgID }}
                  activeOptions={{ exact: true }}
                >
                  <Shield size={17} />
                  Settings & workspaces
                </Link>
                <Link
                  className="nav-link"
                  to="/organizations/$orgId/skills"
                  params={{ orgId: scope.orgID }}
                >
                  <NotebookText size={17} />
                  Organization skills
                </Link>
              </>
            )}
            <Link className="nav-link" to="/skills">
              <NotebookText size={17} />
              Global skills
            </Link>
          </nav>
          <div className="sidebar-footer">
            <a className="nav-link" href="/docs/" target="_blank" rel="noreferrer">
              <CircleHelp size={17} />
              API documentation
            </a>
            <div className="profile">
              <span className="avatar">{session.user.username.slice(0, 2).toUpperCase()}</span>
              <div className="profile-name">
                {session.user.username}
                <p className="muted">
                  {session.platformAdmin ? 'Platform administrator' : 'Signed in'}
                </p>
              </div>
              <Button
                variant="ghost"
                size="icon"
                aria-label="Sign out"
                onClick={() => {
                  void logout();
                }}
              >
                <LogOut size={15} />
              </Button>
            </div>
            {logoutError !== undefined && <ErrorState error={logoutError} />}
          </div>
        </aside>
        <div className="content-shell">
          <header className="topbar">
            <Button
              variant="ghost"
              size="icon"
              className="mobile-toggle"
              aria-label="Open navigation"
              onClick={() => setMobile(!mobile)}
            >
              <Menu size={18} />
            </Button>
            <Link to="/">Orkest</Link>
            {organization.data && (
              <>
                <ChevronRight size={12} />
                <Link to="/organizations/$orgId" params={{ orgId: scope.orgID }}>
                  {organization.data.name}
                </Link>
              </>
            )}
            {workspace.data && (
              <>
                <ChevronRight size={12} />
                <span>{workspace.data.name}</span>
              </>
            )}
            <Badge>{workspace.data?.role ?? organization.data?.role ?? 'Management'}</Badge>
          </header>
          <main id="main-content" className="page">
            {blocked ? (
              <ErrorState
                error={workspace.error ?? organization.error}
                retry={() => {
                  void organization.refetch();
                  if (scope.workspaceID) void workspace.refetch();
                }}
              />
            ) : waiting ? (
              <Loading />
            ) : (
              <Outlet />
            )}
          </main>
        </div>
      </div>
    </AppContext.Provider>
  );
}
