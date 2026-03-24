ALTER TABLE issues ADD COLUMN environment TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN environment TEXT NOT NULL DEFAULT '';
ALTER TABLE issues DROP CONSTRAINT issues_project_id_fingerprint_key;
ALTER TABLE issues ADD CONSTRAINT issues_project_id_fingerprint_environment_key UNIQUE(project_id, fingerprint, environment);
