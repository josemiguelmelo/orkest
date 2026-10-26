import { createContext, useContext } from 'react';
import type { CurrentUser, Organization, Workspace } from '../shared/api/generated/types.gen';
import type { WorkspaceScope } from '../shared/api/queries';
import type { capabilities } from './permissions';

export type AppContext = {
  session: CurrentUser;
  scope: WorkspaceScope;
  organization: Organization | undefined;
  workspace: Workspace | undefined;
  permissions: ReturnType<typeof capabilities>;
};
export const AppContext = createContext<AppContext | null>(null);
export function useApp() {
  const value = useContext(AppContext);
  if (!value) throw new Error('App context is unavailable');
  return value;
}
