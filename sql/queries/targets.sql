-- name: FindTargets :many
SELECT t.id, t.label, t.aspect, t.target_kind
FROM (
    SELECT 
        sqlc.arg(instance_name_id)::bigint AS instance_name_id,
        unnest(sqlc.arg(labels)::text[]) AS label,
        unnest(sqlc.arg(aspects)::text[]) AS aspect,
        unnest(sqlc.arg(target_kinds)::text[]) AS target_kind
) AS input
JOIN targets t ON 
    t.instance_name_targets = input.instance_name_id AND
    t.label = input.label AND
    t.aspect = input.aspect AND
    t.target_kind = input.target_kind;

-- name: CreateTargets :many
INSERT INTO targets (instance_name_targets, label, aspect, target_kind)
SELECT 
    instance_name_id, label, aspect, target_kind
FROM (
    SELECT 
        sqlc.arg(instance_name_id)::bigint AS instance_name_id,
        unnest(sqlc.arg(labels)::text[]) AS label,
        unnest(sqlc.arg(aspects)::text[]) AS aspect,
        unnest(sqlc.arg(target_kinds)::text[]) AS target_kind
) AS input
-- ORDER BY here is enforcing an insertion order for two reasons:
--   1. Prevent concurrent requests from deadlocking each other in case
-- they have overlapping insertions of targets in different orders.
--   2. Explicitly collate to normalize sort order between database
-- instantiations, otherwise golden file generation may have a different
-- order than what's used during the test.
ORDER BY
    label COLLATE "C",
    aspect COLLATE "C",
    target_kind COLLATE "C"
RETURNING id, label, aspect, target_kind;

-- name: DeleteUnusedTargetsFromPages :execrows
DELETE FROM targets
WHERE ctid IN (
    SELECT ctid
    FROM targets
    WHERE
        ctid >= format('(%s,0)', sqlc.arg(from_page)::bigint)::tid
        AND ctid < format('(%s,0)', sqlc.arg(from_page)::bigint + sqlc.arg(pages)::bigint)::tid
        AND (
            NOT EXISTS (
                SELECT 1 FROM "invocation_targets" 
                WHERE "target_invocation_targets" = "targets"."id"
            )
        )
        AND (
            NOT EXISTS (
                SELECT 1 FROM "target_kind_mappings" 
                WHERE "target_id" = "targets"."id"
            )
        )
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(batch_limit)::bigint
);

-- name: UpdateTargetsMaxTestRunDuration :exec
-- Raises max_test_run_duration_in_ms for the targets that have a test summary
-- in the given invocation. It never lowers the value, so saving a shorter run
-- does not forget a longer earlier one.
UPDATE targets t
SET max_test_run_duration_in_ms = source.max_duration
FROM (
    SELECT
        it.target_invocation_targets AS target_id,
        MAX(ts.total_run_duration_in_ms) AS max_duration
    FROM invocation_targets it
    JOIN test_summaries ts
        ON ts.invocation_target_test_summary = it.id
    WHERE it.bazel_invocation_invocation_targets = sqlc.arg(bazel_invocation_id)
        AND ts.total_run_duration_in_ms IS NOT NULL
    GROUP BY it.target_invocation_targets
) AS source
WHERE t.id = source.target_id
    AND t.max_test_run_duration_in_ms < source.max_duration;
