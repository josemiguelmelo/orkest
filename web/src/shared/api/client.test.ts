import { afterAll, afterEach, beforeAll, describe, it, expect } from 'vitest';
import { http, HttpResponse } from 'msw';
import { setupServer } from 'msw/node';
import { session } from '../../test/fixtures';
import { command, data, ApiError } from './client';
import { getCurrentUser } from './generated/sdk.gen';
const server = setupServer();
beforeAll(() => server.listen());
afterEach(() => server.resetHandlers());
afterAll(() => server.close());
describe('validated API boundary', () => {
  it('accepts a real contract-shaped response', async () => {
    server.use(http.get('*/api/me', () => HttpResponse.json(session)));
    expect(await data(getCurrentUser(command))).toEqual(session);
  });
  it('rejects malformed server data at runtime', async () => {
    server.use(
      http.get('*/api/me', () => HttpResponse.json({ user: 'invalid', organizations: [] })),
    );
    await expect(data(getCurrentUser(command))).rejects.toThrow('API contract');
  });
  it('sanitizes raw error bodies', async () => {
    server.use(
      http.get('*/api/me', () => new HttpResponse('credential=do-not-show', { status: 500 })),
    );
    try {
      await data(getCurrentUser(command));
      expect.fail('must reject');
    } catch (error) {
      expect(error).toBeInstanceOf(ApiError);
      expect(String(error)).not.toContain('do-not-show');
    }
  });
  it('announces session expiry', async () => {
    server.use(
      http.get('*/api/me', () => HttpResponse.json({ error: 'unauthorized' }, { status: 401 })),
    );
    let expired = false;
    const listener = () => {
      expired = true;
    };
    window.addEventListener('orkest:session-expired', listener);
    await expect(data(getCurrentUser(command))).rejects.toThrow('session has expired');
    expect(expired).toBe(true);
    window.removeEventListener('orkest:session-expired', listener);
  });
});
