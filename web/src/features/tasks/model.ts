import { z } from 'zod';
import type { Task, TaskInput } from '../../shared/api/generated/types.gen';
export const statuses = ['todo', 'in_progress', 'in_review', 'done'] as const;
export const statusNames: Record<Task['status'], string> = {
  todo: 'Todo',
  in_progress: 'In progress',
  in_review: 'In review',
  done: 'Done',
};
export const taskFormSchema = z.object({
  title: z.string().trim().min(1, 'Enter a title.').max(500),
  description: z.string().max(100000),
  status: z.enum(statuses),
  assigneeAgentId: z.string(),
  labels: z.string(),
  componentIds: z.array(z.string()),
});
export type TaskFormValues = z.infer<typeof taskFormSchema>;
export function taskInput(values: TaskFormValues, current?: Task): TaskInput {
  if (!current && values.status !== 'todo') throw new Error('New tasks must start in todo.');
  if (current?.status === 'done' && values.status !== 'done')
    throw new Error('Done tasks cannot be reopened.');
  return {
    title: current && current.sourceProvider !== 'manual' ? current.title : values.title,
    description:
      current && current.sourceProvider !== 'manual' ? current.description : values.description,
    status: values.status,
    assigneeAgentId: values.assigneeAgentId,
    labels:
      current && current.sourceProvider !== 'manual'
        ? current.labels
        : values.labels
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean),
    componentIds: values.componentIds,
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
