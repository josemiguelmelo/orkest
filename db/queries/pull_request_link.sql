-- name: NextUnlinkedPullRequest :one
SELECT d.* FROM task_trigger_deliveries d WHERE status='awaiting_link' AND resolution_after<=now()
ORDER BY resolution_after,id LIMIT 1 FOR UPDATE SKIP LOCKED;

-- name: PullRequestBranchCandidates :many
WITH snapshots AS (
 SELECT DISTINCT ON(l.task_id,e.event->'repositoryObservation'->'checkout'->>'path')
 l.task_id,e.event->'repositoryObservation' AS snapshot
 FROM agent_launch_events e JOIN agent_launches l ON l.run_id=e.run_id
 WHERE l.workspace_id=$1 AND e.event->>'type'='repository_observed'
 ORDER BY l.task_id,e.event->'repositoryObservation'->'checkout'->>'path', (e.event->>'at')::timestamptz DESC, e.sequence DESC
)
SELECT s.task_id,r.provider_repository_id AS repository_id,(b.value->>'name')::text AS branch,(b.value->>'commitId')::text AS commit_id,
COALESCE(r.default_branch,'')::text AS default_branch,COALESCE(s.snapshot->>'diagnostic','')::text AS diagnostic,
(COALESCE(b.value->>'eligibleForLinking','false')='true') AS eligible_for_linking
FROM snapshots s
JOIN task_repositories tr ON tr.workspace_id=$1 AND tr.task_id=s.task_id AND tr.repository_id=s.snapshot->'checkout'->>'repositoryId'
JOIN code_repositories r ON r.workspace_id=tr.workspace_id AND r.id=tr.repository_id
CROSS JOIN LATERAL jsonb_array_elements(COALESCE(NULLIF(s.snapshot->'branches','null'::jsonb),'[]'::jsonb)) b
WHERE r.provider_name=$2 AND r.provider_repository_id=$3
ORDER BY s.task_id,branch,commit_id;

-- name: AssociatePullRequest :execrows
INSERT INTO task_pull_request_links(workspace_id,provider,repository_id,resource_id,task_id,head_repository_id,head_branch)
VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(workspace_id,provider,repository_id,resource_id) DO UPDATE SET head_branch=task_pull_request_links.head_branch
WHERE task_pull_request_links.task_id=EXCLUDED.task_id;

-- name: ExistingPullRequestTask :one
SELECT task_id FROM task_pull_request_links WHERE workspace_id=$1 AND provider=$2 AND repository_id=$3 AND resource_id=$4;

-- name: PullRequestRepositoryRegistered :one
SELECT EXISTS(SELECT 1 FROM code_repositories
WHERE workspace_id=$1 AND provider_name=$2 AND provider_repository_id=$3);

-- name: SavePullRequestResolution :execrows
UPDATE task_trigger_deliveries SET task_id=$2,stage_version=$3,status=$4,diagnostic=$5,resolution_after=COALESCE(sqlc.narg(retry_at)::timestamptz,resolution_after)
WHERE id=$1 AND status=sqlc.arg(expected_status);

-- name: TaskTriggerLinkSuppressed :one
SELECT EXISTS(SELECT 1 FROM task_pull_request_suppressions WHERE integration_id=$1 AND repository_id=$2 AND resource_id=$3);

-- name: LockPullRequestWorkspace :exec
SELECT pg_advisory_xact_lock(hashtextextended($1::text, 73483));

-- name: InitializePullRequestDiscovery :exec
INSERT INTO task_pull_request_reconciliation(integration_id) SELECT id FROM task_trigger_integrations WHERE enabled ON CONFLICT DO NOTHING;

-- name: ClaimPullRequestDiscovery :one
UPDATE task_pull_request_reconciliation s SET next_attempt=now()+interval '60 seconds',
 activated_at=CASE WHEN s.configuration_version=i.version THEN s.activated_at ELSE now() END, configuration_version=i.version
 FROM task_trigger_integrations i
WHERE s.integration_id=i.id AND s.integration_id=(SELECT s.integration_id FROM task_pull_request_reconciliation s
JOIN task_trigger_integrations i ON i.id=s.integration_id WHERE i.enabled AND s.next_attempt<=now()
ORDER BY s.next_attempt,s.integration_id LIMIT 1 FOR UPDATE OF s SKIP LOCKED)
RETURNING s.integration_id,s.activated_at;

-- name: FinishPullRequestDiscovery :exec
UPDATE task_pull_request_reconciliation SET diagnostic=$2 WHERE integration_id=$1;

-- name: PullRequestDiscoveryRepositories :many
SELECT DISTINCT r.* FROM code_repositories r JOIN task_repositories tr ON tr.workspace_id=r.workspace_id AND tr.repository_id=r.id
WHERE r.workspace_id=$1 AND r.provider_name=$2;

-- name: PullRequestDiscoveryDiagnostic :one
SELECT diagnostic FROM task_pull_request_reconciliation WHERE integration_id=$1;

-- name: ResetPullRequestDiscovery :exec
INSERT INTO task_pull_request_reconciliation(integration_id,configuration_version)
SELECT id,version FROM task_trigger_integrations WHERE id=$1
ON CONFLICT(integration_id) DO UPDATE SET configuration_version=EXCLUDED.configuration_version,activated_at=now(),next_attempt=now(),diagnostic='';

-- name: PullRequestTaskStates :many
SELECT t.id,t.stage_version,
 EXISTS(SELECT 1 FROM task_repositories tr JOIN code_repositories r ON r.workspace_id=tr.workspace_id AND r.id=tr.repository_id
 WHERE tr.workspace_id=t.workspace_id AND tr.task_id=t.id AND r.provider_name=$2 AND r.provider_repository_id=$3) AS repository_selected,
 (SELECT max(e.occurred_at) FROM workflow_events e WHERE e.workspace_id=t.workspace_id AND e.task_id=t.id AND e.event_type='stage_entered' AND e.stage_version=t.stage_version)::timestamptz AS stage_entered_at
FROM tasks t WHERE t.workspace_id=$1 ORDER BY t.id;

-- name: LockObservationLaunches :exec
SELECT run_id FROM agent_launches WHERE workspace_id=$1 ORDER BY run_id FOR UPDATE;

-- name: LockAssociationTasks :exec
SELECT id FROM tasks WHERE workspace_id=$1 ORDER BY id FOR UPDATE;

-- name: PullRequestEventResolution :one
SELECT * FROM task_trigger_deliveries
WHERE integration_id=$1 AND event_key=$2 AND status='awaiting_link';
