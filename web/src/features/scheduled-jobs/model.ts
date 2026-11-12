import type { ScheduledJobInput } from '../../shared/api/generated/types.gen';
// Keep server-owned metadata outside mutation payloads (the API rejects unknown fields).
export function jobInput(job: ScheduledJobInput): ScheduledJobInput {
  return {
    name: job.name,
    instructions: job.instructions,
    agentId: job.agentId,
    repositoryScope: job.repositoryScope,
    repositoryIds: job.repositoryIds,
    schedule: job.schedule,
    enabled: job.enabled,
    maxDurationMinutes: job.maxDurationMinutes,
  };
}
