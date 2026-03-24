DELETE FROM issues a USING issues b
  WHERE a.project_id = b.project_id AND a.fingerprint = b.fingerprint AND a.id < b.id;

ALTER TABLE issues DROP CONSTRAINT issues_project_id_fingerprint_environment_key;
ALTER TABLE issues ADD CONSTRAINT issues_project_id_fingerprint_key UNIQUE(project_id, fingerprint);
ALTER TABLE issues DROP COLUMN environment;
ALTER TABLE events DROP COLUMN environment;
