import type { Organization, Workspace, Skill } from '../shared/api/generated/types.gen';
export function capabilities(
  orgRole: Organization['role'],
  workspaceRole: Workspace['role'],
  platformAdmin: boolean,
) {
  const organizationAdmin = orgRole === 'owner' || orgRole === 'admin';
  const workspaceAdmin =
    organizationAdmin || workspaceRole === 'admin' || workspaceRole === 'owner';
  return {
    organizationAdmin,
    workspaceAdmin,
    editWorkspace: workspaceAdmin || workspaceRole === 'member',
    platformAdmin,
  };
}
export function canManageSkill(
  skill: Pick<Skill, 'scope' | 'organizationId' | 'workspaceId'>,
  context: { orgID: string; workspaceID: string; permissions: ReturnType<typeof capabilities> },
) {
  if (skill.scope === 'global') return context.permissions.platformAdmin;
  if (skill.scope === 'organization')
    return skill.organizationId === context.orgID && context.permissions.organizationAdmin;
  return skill.workspaceId === context.workspaceID && context.permissions.workspaceAdmin;
}
