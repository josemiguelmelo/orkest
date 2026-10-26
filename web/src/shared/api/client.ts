import { z } from 'zod';
import { client } from './generated/client.gen';

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
  ) {
    super(
      messages[code] ??
        (status === 404
          ? 'This resource is unavailable or you no longer have access.'
          : status === 403
            ? 'You do not have permission for this action.'
            : status === 401
              ? 'Your session has expired. Sign in to continue.'
              : 'The request could not be completed. Refresh before trying again.'),
    );
    this.name = 'ApiError';
  }
}
const messages: Record<string, string> = {
  invalid_request: 'Some values are invalid. Check the form and try again.',
  conflict:
    'This change conflicts with the current state. The last owner cannot be removed, and done tasks cannot reopen.',
  integration_encryption_not_configured: 'Credential encryption is not configured on the server.',
  integration_credential_required: 'A credential is required to validate this connection.',
  github_project_not_found_or_not_accessible:
    'The GitHub project is unavailable with these credentials.',
  agent_dispatch_failed:
    'The task changed to in progress, but agent dispatch failed. Refresh to see its current state.',
  run_not_found: 'This run is no longer available. Refresh the task to load its current run history.',
  run_activity_unavailable:
    'Agent activity could not be loaded right now. Try again in a moment.',
};
const errorSchema = z.object({ error: z.string() });
client.setConfig({ baseUrl: window.location.origin, credentials: 'same-origin' });
client.interceptors.error.use((error, response) => {
  if (response && !response.ok) {
    const parsed = errorSchema.safeParse(error);
    if (response.status === 401) window.dispatchEvent(new Event('orkest:session-expired'));
    return new ApiError(response.status, parsed.success ? parsed.data.error : 'request_failed');
  }
  if (error instanceof z.ZodError)
    return new Error(
      'The server response does not match the API contract. Refresh or contact your administrator.',
    );
  return new Error('Cannot reach the server. Check your connection and refresh before retrying.');
});
export async function data<T>(request: Promise<{ data: T }>): Promise<T> {
  return (await request).data;
}
export const command = { throwOnError: true } as const;
