import { describe, it, expect } from 'vitest';
import { capabilities, canManageSkill } from './permissions';
describe('capabilities', () => {
  it('keeps viewers read-only', () => {
    const p = capabilities('member', 'viewer', false);
    expect(p.editWorkspace).toBe(false);
    expect(p.workspaceAdmin).toBe(false);
  });
  it('inherits organization administrators without granting global administration', () => {
    const p = capabilities('owner', undefined, false);
    expect(p.workspaceAdmin).toBe(true);
    expect(p.editWorkspace).toBe(true);
    expect(p.platformAdmin).toBe(false);
  });
  it('allows members to edit resources but not administrator configuration', () => {
    const p = capabilities('member', 'member', false);
    expect(p.editWorkspace).toBe(true);
    expect(p.workspaceAdmin).toBe(false);
  });
  it('checks the actual scope of inherited skills', () => {
    const context = {
      orgID: 'org-1',
      workspaceID: 'ws-1',
      permissions: capabilities('member', 'admin', false),
    };
    expect(canManageSkill({ scope: 'global' }, context)).toBe(false);
    expect(canManageSkill({ scope: 'organization', organizationId: 'org-1' }, context)).toBe(false);
    expect(canManageSkill({ scope: 'workspace', workspaceId: 'ws-1' }, context)).toBe(true);
    expect(canManageSkill({ scope: 'workspace', workspaceId: 'ws-other' }, context)).toBe(false);
  });
});
