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
  question_unavailable:
    'This question has closed or cannot receive an answer. Refresh the conversation before retrying.',
  invalid_question: 'Check the question and your answer, then try again.',
  invalid_answer: 'Your answer does not match the requested fields.',

  execution_images_not_configured:
    'Image building is not configured. Ask your administrator to set ORKEST_EXECUTION_IMAGE_REPOSITORY to a registry repository.',
  invalid_request: 'Some values are invalid. Check the form and try again.',
  conflict:
    'This item is still in use, or its current state prevents this change. Remove dependent resources and try again.',
  integration_encryption_not_configured: 'Credential encryption is not configured on the server.',
  integration_credential_required: 'A credential is required to validate this connection.',
  integration_validation_failed:
    'The source could not be reached or rejected the supplied connection details. Check the URL, credential, and project access, then retry.',
  github_config_invalid: 'The GitHub source configuration is invalid. Check the project URL.',
  github_project_url_required: 'Enter a GitHub Projects URL.',
  github_project_url_invalid:
    'Use a GitHub Projects v2 URL such as https://github.com/orgs/OWNER/projects/1.',
  integration_sync_failed:
    'The sync failed while saving imported tasks. Retry, and contact your administrator if it keeps failing.',
  github_api_unavailable: 'Could not reach GitHub. Check the server network connection and retry.',
  github_project_response_invalid: 'GitHub returned an unexpected response. Retry later.',
  github_project_permission_denied:
    'GitHub denied access to this project. Confirm the token can read Projects and has access to the project.',
  github_graphql_query_failed:
    'GitHub could not read this project. Confirm the token permissions and project configuration.',
  github_token_invalid: 'The GitHub token is invalid or revoked. Replace it in workspace settings.',
  github_api_status_401:
    'The GitHub token is invalid or revoked. Replace it in workspace settings.',
  github_api_status_403:
    'GitHub denied access or rate limited the request. Check token permissions and retry later.',
  github_api_status_429: 'GitHub rate limited the request. Retry later.',
  github_api_status_500: 'GitHub is temporarily unavailable. Retry later.',
  github_api_status_502: 'GitHub is temporarily unavailable. Retry later.',
  github_api_status_503: 'GitHub is temporarily unavailable. Retry later.',
  github_project_not_found_or_not_accessible:
    'The GitHub project is unavailable with these credentials.',
  github_repository_unavailable:
    'This repository is no longer accessible to the selected integration.',
  github_connection_failed:
    'Could not validate the GitHub connection. Check the token and try again.',
  gitlab_config_invalid: 'The GitLab connection settings are invalid. Check the server URL.',
  gitlab_token_required: 'Enter a GitLab personal access token.',
  gitlab_authentication_failed: 'GitLab rejected this token. Check it and try again.',
  gitlab_connection_failed:
    'Could not validate the GitLab connection. Check the URL, token, and server availability.',
  gitlab_token_invalid_or_revoked:
    'The GitLab token is invalid or revoked. Replace it in workspace settings.',
  gitlab_permission_denied: 'GitLab denied access. Check token permissions and project access.',
  gitlab_repository_unavailable:
    'This repository is no longer accessible to the selected integration.',
  agent_dispatch_failed:
    'The task changed to in progress, but agent dispatch failed. Refresh to see its current state.',
  run_not_found:
    'This run is no longer available. Refresh the task to load its current run history.',
  run_activity_unavailable: 'Agent activity could not be loaded right now. Try again in a moment.',
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
