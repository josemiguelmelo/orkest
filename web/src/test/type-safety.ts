// These compile-only assertions fail if the application loses its contracts.
import type { TaskInput } from '../shared/api/generated/types.gen';
import { router } from '../app/router';
import { taskInput } from '../features/tasks/model';
// @ts-expect-error task stage IDs are strings
const invalid: TaskInput = { title: 'Example', status: 123 };
function invalidNavigation() {
  // @ts-expect-error route parameters must match the route
  void router.navigate({ to: '/organizations/$orgId', params: { workspaceId: 'wrong' } });
}
function unvalidated(value: unknown) {
  // @ts-expect-error unknown external data must be parsed before use
  return taskInput(value);
}
void invalid;
void invalidNavigation;
void unvalidated;
