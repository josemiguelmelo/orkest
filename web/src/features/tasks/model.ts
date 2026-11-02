import { z } from 'zod';
import type { Task, TaskInput } from '../../shared/api/generated/types.gen';
export const taskFormSchema = z.object({
  title: z.string().trim().min(1, 'Enter a title.').max(500),
  description: z.string().max(100000),
  workflowId: z.string().min(1),
  status: z.string().min(1),
  assigneeAgentId: z.string(),
  labels: z.string(),
  repositoryIds: z.array(z.string()),
});
export type TaskFormValues = z.infer<typeof taskFormSchema>;
export function taskInput(values: TaskFormValues, current?: Task): TaskInput {
  if (current?.closed && values.status !== current.status)
    throw new Error('Done tasks cannot be reopened.');
  return {
    title: current && current.sourceProvider !== 'manual' ? current.title : values.title,
    description:
      current && current.sourceProvider !== 'manual' ? current.description : values.description,
    status: values.status,
    workflowId: current?.workflowId ?? values.workflowId,
    ...(current?.stageVersion ? { stageVersion: current.stageVersion } : {}),
    assigneeAgentId: values.assigneeAgentId,
    labels:
      current && current.sourceProvider !== 'manual'
        ? current.labels
        : values.labels
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean),
    repositoryIds: values.repositoryIds,
  };
}
export function filterTasks(
  tasks: Task[],
  search: { q: string; status: string; agent: string; sort: string },
) {
  return tasks
    .filter(
      (t) =>
        (!search.q ||
          `${t.title} ${t.description} ${t.labels.join(' ')}`
            .toLowerCase()
            .includes(search.q.toLowerCase())) &&
        (search.status === 'all' || t.status === search.status) &&
        (!search.agent || t.assigneeAgentId === search.agent),
    )
    .sort((a, b) =>
      search.sort === 'title'
        ? a.title.localeCompare(b.title)
        : b.updatedAt.localeCompare(a.updatedAt),
    );
}
