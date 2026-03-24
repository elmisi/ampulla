-- Merge issues that share (project_id, fingerprint) but differ by environment.
-- Keep the one with the most events, reassign orphaned events, delete the rest.
WITH keep AS (
    SELECT DISTINCT ON (project_id, fingerprint)
        id, project_id, fingerprint
    FROM issues
    ORDER BY project_id, fingerprint, event_count DESC, id
),
duplicates AS (
    SELECT i.id AS dup_id, k.id AS keep_id
    FROM issues i
    JOIN keep k ON k.project_id = i.project_id AND k.fingerprint = i.fingerprint
    WHERE i.id != k.id
)
UPDATE events SET issue_id = d.keep_id FROM duplicates d WHERE events.issue_id = d.dup_id;

DELETE FROM issues WHERE id NOT IN (
    SELECT DISTINCT ON (project_id, fingerprint) id
    FROM issues
    ORDER BY project_id, fingerprint, event_count DESC, id
);

ALTER TABLE issues DROP CONSTRAINT issues_project_id_fingerprint_environment_key;
ALTER TABLE issues ADD CONSTRAINT issues_project_id_fingerprint_key UNIQUE(project_id, fingerprint);
ALTER TABLE issues DROP COLUMN environment;
ALTER TABLE events DROP COLUMN environment;
