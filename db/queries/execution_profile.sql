-- name: ListExecutionProfiles :many
SELECT * FROM execution_profiles WHERE workspace_id=$1 ORDER BY name,id;
-- name: GetExecutionProfile :one
SELECT * FROM execution_profiles WHERE workspace_id=$1 AND id=$2;
-- name: InsertExecutionProfile :one
INSERT INTO execution_profiles(id,workspace_id,name,description,runtime,toolchain_image) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;
-- name: UpdateExecutionProfile :one
UPDATE execution_profiles SET name=$3,description=$4,toolchain_image=$5,generation=generation+1,status='queued',build_error='',updated_at=now()
WHERE workspace_id=$1 AND id=$2 AND generation=$6 RETURNING *;
-- name: InsertExecutionBuild :exec
INSERT INTO execution_profile_builds(id,workspace_id,profile_id,generation,runtime,toolchain_image,base_image,destination,runtime_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);
-- name: ListExecutionBuilds :many
SELECT * FROM execution_profile_builds WHERE workspace_id=$1 AND profile_id=$2 ORDER BY created_at DESC,id LIMIT 100;
-- name: ClaimExecutionBuild :one
WITH candidate AS (
 SELECT id,status FROM execution_profile_builds
 WHERE status='queued' OR (status IN ('building','reconciling') AND lease_until<now())
 ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1
)
UPDATE execution_profile_builds b SET status=CASE WHEN c.status='queued' THEN 'building' ELSE 'reconciling' END,owner=$1,lease_until=now()+interval '90 seconds'
FROM candidate c WHERE b.id=c.id RETURNING b.*;
-- name: HeartbeatExecutionBuild :execrows
UPDATE execution_profile_builds SET lease_until=now()+interval '90 seconds' WHERE id=$1 AND owner=$2 AND status IN ('building','reconciling');
-- name: FinishExecutionBuild :one
UPDATE execution_profile_builds SET status=$3,execution_image=$4,build_error=$5,lease_until=NULL
WHERE id=$1 AND owner=$2 AND status IN ('building','reconciling') RETURNING *;
-- name: PromoteExecutionBuild :exec
UPDATE execution_profiles SET status=$3,execution_image=CASE WHEN $3='ready' THEN $4 ELSE execution_image END,build_error=$5,updated_at=now()
WHERE id=$1 AND generation=$2;

-- name: PinExecutionBuildInputs :execrows
UPDATE execution_profile_builds SET resolved_toolchain=$3,resolved_base=$4 WHERE id=$1 AND owner=$2 AND status='building';
