ALTER TABLE events DROP CONSTRAINT events_issue_id_fkey;
ALTER TABLE events ADD CONSTRAINT events_issue_id_fkey FOREIGN KEY (issue_id) REFERENCES issues(id);
