-- name: ListScheduledJobs :many
SELECT j.* FROM scheduled_jobs j JOIN workspaces w ON w.id=j.workspace_id
WHERE w.organization_id=$1 AND j.workspace_id=$2 ORDER BY j.created_at,j.id;

-- name: GetScheduledJob :one
SELECT j.* FROM scheduled_jobs j JOIN workspaces w ON w.id=j.workspace_id
WHERE w.organization_id=$1 AND j.workspace_id=$2 AND j.id=$3;

-- name: LockScheduledJob :one
SELECT j.* FROM scheduled_jobs j JOIN workspaces w ON w.id=j.workspace_id
WHERE w.organization_id=$1 AND j.workspace_id=$2 AND j.id=$3 FOR UPDATE OF j;

-- name: InsertScheduledJob :execrows
INSERT INTO scheduled_jobs(id,workspace_id,authorized_by,definition,enabled,next_due_at)
SELECT $1,w.id,$3,$4,$5,$6 FROM workspaces w WHERE w.id=$2 AND w.organization_id=$7 ON CONFLICT(id) DO NOTHING;

-- name: UpdateScheduledJob :execrows
UPDATE scheduled_jobs SET revision=revision+1,authorized_by=$3,definition=$4,enabled=$5,next_due_at=$6
WHERE workspace_id=$1 AND id=$2 AND revision=$7;

-- name: InsertScheduledJobRevision :exec
INSERT INTO scheduled_job_revisions(workspace_id,job_id,revision,authorized_by,definition) VALUES($1,$2,$3,$4,$5);

-- name: GetScheduledJobRevision :one
SELECT * FROM scheduled_job_revisions WHERE workspace_id=$1 AND job_id=$2 AND revision=$3;

-- name: PendingScheduledJob :one
SELECT sqlc.embed(j),w.organization_id,now()::timestamptz AS database_now FROM scheduled_jobs j JOIN workspaces w ON w.id=j.workspace_id
WHERE j.enabled AND j.next_due_at<=now() ORDER BY j.next_due_at,j.id LIMIT 1 FOR UPDATE OF j SKIP LOCKED;

-- name: AdvanceScheduledJob :exec
UPDATE scheduled_jobs SET next_due_at=$3 WHERE workspace_id=$1 AND id=$2;

-- name: DisableScheduledJob :exec
UPDATE scheduled_jobs SET enabled=false WHERE workspace_id=$1 AND id=$2;

-- name: ScheduledJobRepositoryIDs :many
SELECT id FROM code_repositories WHERE workspace_id=$1 ORDER BY id;

-- name: InsertScheduledJobRun :exec
INSERT INTO scheduled_job_runs(id,workspace_id,job_id,revision,actor_id,definition,repository_ids,scheduled_for,trigger_key,state,reason,run_id,session_id,command_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14);

-- name: GetScheduledJobRun :one
SELECT sqlc.embed(r),w.organization_id FROM scheduled_job_runs r JOIN workspaces w ON w.id=r.workspace_id
WHERE w.organization_id=$1 AND r.workspace_id=$2 AND r.job_id=$3 AND r.id=$4;

-- name: FindScheduledJobTrigger :one
SELECT sqlc.embed(r),w.organization_id FROM scheduled_job_runs r JOIN workspaces w ON w.id=r.workspace_id
WHERE w.organization_id=$1 AND r.workspace_id=$2 AND r.job_id=$3 AND r.trigger_key=$4;

-- name: ListScheduledJobRuns :many
SELECT sqlc.embed(r),w.organization_id FROM scheduled_job_runs r JOIN workspaces w ON w.id=r.workspace_id
WHERE w.organization_id=$1 AND r.workspace_id=$2 AND r.job_id=$3
ORDER BY r.created_at DESC,r.id DESC LIMIT $4 OFFSET $5;

-- name: ScheduledJobOccupied :one
SELECT EXISTS(SELECT 1 FROM scheduled_job_runs WHERE workspace_id=$1 AND job_id=$2 AND state IN ('pending','starting','running','delivery_uncertain'));

-- name: PendingScheduledJobRun :one
SELECT sqlc.embed(r),w.organization_id,now()::timestamptz AS database_now FROM scheduled_job_runs r JOIN workspaces w ON w.id=r.workspace_id
WHERE (r.state='pending' AND (r.created_at<now()-interval '1 hour' OR NOT EXISTS(
 SELECT 1 FROM scheduled_job_runs busy WHERE busy.workspace_id=r.workspace_id AND busy.id<>r.id AND busy.state IN ('starting','running','delivery_uncertain'))))
OR (r.state='starting' AND r.claim_until<=now())
ORDER BY r.created_at,r.id LIMIT 1 FOR UPDATE OF r SKIP LOCKED;

-- name: LockScheduledJobWorkspace :one
SELECT id FROM workspaces WHERE id=$1 FOR UPDATE;

-- name: ScheduledWorkspaceOccupied :one
SELECT EXISTS(SELECT 1 FROM scheduled_job_runs WHERE workspace_id=$1 AND id<>$2 AND state IN ('starting','running','delivery_uncertain'));

-- name: ClaimScheduledJobRun :exec
UPDATE scheduled_job_runs SET state='starting',claim_token=$2,claim_attempt=claim_attempt+1,claim_until=now()+interval '30 seconds' WHERE id=$1;

-- name: LockScheduledJobClaim :one
SELECT * FROM scheduled_job_runs WHERE id=$1 AND claim_token=$2 AND state='starting' FOR UPDATE;

-- name: FinishScheduledJobClaim :execrows
UPDATE scheduled_job_runs SET state=$3,reason=$4,claim_until=NULL WHERE id=$1 AND claim_token=$2 AND state='starting';

-- name: CancelPendingScheduledJobRun :execrows
UPDATE scheduled_job_runs SET cancel_requested=true,state=CASE WHEN state IN ('pending','starting','delivery_uncertain')
 AND NOT EXISTS(SELECT 1 FROM agent_launches l WHERE l.run_id=scheduled_job_runs.run_id) THEN 'cancelled' ELSE state END
WHERE scheduled_job_runs.workspace_id=$1 AND scheduled_job_runs.job_id=$2 AND scheduled_job_runs.id=$3;

-- name: IsScheduledJobRun :one
SELECT EXISTS(SELECT 1 FROM scheduled_job_runs WHERE run_id=$1);

-- name: FinishScheduledJobLaunch :execrows
UPDATE scheduled_job_runs SET state=CASE WHEN cancel_requested THEN 'cancelled' WHEN sqlc.arg(status)::text='completed' THEN 'succeeded' ELSE 'failed' END,claim_until=NULL
WHERE run_id=sqlc.arg(run_id) AND sqlc.arg(status)::text IN ('completed','failed');

-- name: ExpiredScheduledJobRuns :many
SELECT sqlc.embed(r),w.organization_id FROM scheduled_job_runs r JOIN workspaces w ON w.id=r.workspace_id JOIN agent_launches l ON l.run_id=r.run_id
WHERE r.state IN ('starting','running') AND (r.cancel_requested OR l.created_at+((r.definition->>'maxDurationMinutes')::int*interval '1 minute')<=now())
ORDER BY r.created_at LIMIT 20;

-- name: MarkScheduledJobTimeout :exec
UPDATE scheduled_job_runs SET reason='maximum_duration_exceeded' WHERE id=$1 AND NOT cancel_requested;

-- name: ScheduledJobCanMutate :one
SELECT EXISTS(SELECT 1 FROM organization_memberships om JOIN workspaces w ON w.organization_id=om.organization_id
 WHERE w.id=$1 AND om.user_id=$2 AND (om.role IN ('owner','admin') OR EXISTS(
 SELECT 1 FROM workspace_memberships wm WHERE wm.workspace_id=w.id AND wm.user_id=om.user_id AND wm.role IN ('admin','member'))));

-- name: LockScopedScheduledJobRun :one
SELECT r.id FROM scheduled_job_runs r JOIN workspaces w ON w.id=r.workspace_id
WHERE w.organization_id=$1 AND r.workspace_id=$2 AND r.job_id=$3 AND r.id=$4 FOR UPDATE OF r;

-- name: ScheduledJobAdmissible :one
SELECT r.created_at>=now()-interval '1 hour' AND NOT EXISTS(
 SELECT 1 FROM scheduled_job_runs busy WHERE busy.workspace_id=r.workspace_id AND busy.id<>r.id
 AND busy.state IN ('starting','running','delivery_uncertain')) FROM scheduled_job_runs r WHERE r.id=$1;

-- name: ScheduledJobAgentExists :one
SELECT EXISTS(SELECT 1 FROM agents WHERE workspace_id=$1 AND id=$2);
