import { describe, it, expect } from 'vitest';
import { task } from '../../test/fixtures';
import { filterTasks, taskInput, type TaskFormValues } from './model';
const values: TaskFormValues = {
  title: 'Edited',
  description: 'Changed',
  workflowId: 'workspace',
  status: 'todo',
  assigneeAgentId: '',
  labels: 'new',
  repositoryIds: [],
};
describe('task updates', () => {
  it('accepts configured stage IDs', () => {
    expect(taskInput({ ...values, status: 'custom-stage' }).status).toBe('custom-stage');
    expect(taskInput({ ...values, status: 'in_progress' }, task).status).toBe('in_progress');
    expect(
      taskInput({ ...values, status: 'in_progress' }, { ...task, status: 'in_progress' }).status,
    ).toBe('in_progress');
  });
  it('preserves every source-managed field on an imported task', () => {
    const imported = { ...task, sourceProvider: 'github_cloud' };
    const input = taskInput({ ...values, status: 'in_review' }, imported);
    expect(input.title).toBe(task.title);
    expect(input.description).toBe(task.description);
    expect(input.labels).toEqual(task.labels);
    expect(input.status).toBe('in_review');
  });
  it('does not reopen done tasks', () => {
    expect(() => taskInput(values, { ...task, status: 'done', closed: true })).toThrow(
      'cannot be reopened',
    );
  });
  it('filters by text, status and agent together', () => {
    expect(
      filterTasks([task], { q: 'WORKBENCH', status: 'todo', agent: 'agent-1', sort: 'title' }),
    ).toEqual([task]);
    expect(filterTasks([task], { q: '', status: 'done', agent: '', sort: 'title' })).toEqual([]);
  });
});
