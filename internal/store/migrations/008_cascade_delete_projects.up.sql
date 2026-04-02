-- Add ON DELETE CASCADE to all foreign keys referencing projects(id)
-- so that deleting a project removes all its dependent data.

-- project_keys.project_id
ALTER TABLE project_keys DROP CONSTRAINT project_keys_project_id_fkey;
ALTER TABLE project_keys ADD CONSTRAINT project_keys_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

-- issues.project_id
ALTER TABLE issues DROP CONSTRAINT issues_project_id_fkey;
ALTER TABLE issues ADD CONSTRAINT issues_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

-- events.project_id
ALTER TABLE events DROP CONSTRAINT events_project_id_fkey;
ALTER TABLE events ADD CONSTRAINT events_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

-- transactions.project_id
ALTER TABLE transactions DROP CONSTRAINT transactions_project_id_fkey;
ALTER TABLE transactions ADD CONSTRAINT transactions_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

-- Also add CASCADE on projects.org_id for organization deletion
ALTER TABLE projects DROP CONSTRAINT projects_org_id_fkey;
ALTER TABLE projects ADD CONSTRAINT projects_org_id_fkey
    FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE;
