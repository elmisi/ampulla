-- Create ntfy_configurations table
CREATE TABLE ntfy_configurations (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    url        TEXT NOT NULL,
    topic      TEXT NOT NULL,
    token      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Unique index on the logical triplet (url, topic, token) with NULL-safe token comparison
CREATE UNIQUE INDEX idx_ntfy_configurations_unique_triplet
ON ntfy_configurations (url, topic, COALESCE(token, ''));

-- Add FK column to projects
ALTER TABLE projects
ADD COLUMN ntfy_config_id BIGINT REFERENCES ntfy_configurations(id) ON DELETE SET NULL;

-- Migrate existing inline ntfy config into the new table.
-- For each distinct (url, topic, token) triplet, create one configuration.
-- Name defaults to topic; collisions get " (2)", " (3)" etc.
DO $$
DECLARE
    rec RECORD;
    config_id BIGINT;
    base_name TEXT;
    try_name TEXT;
    suffix INT;
BEGIN
    FOR rec IN
        SELECT DISTINCT ntfy_url, ntfy_topic, ntfy_token
        FROM projects
        WHERE ntfy_url IS NOT NULL AND ntfy_url != ''
          AND ntfy_topic IS NOT NULL AND ntfy_topic != ''
    LOOP
        -- Check if this triplet already exists (idempotent)
        SELECT id INTO config_id
        FROM ntfy_configurations
        WHERE url = rec.ntfy_url
          AND topic = rec.ntfy_topic
          AND COALESCE(token, '') = COALESCE(rec.ntfy_token, '');

        IF config_id IS NULL THEN
            -- Generate unique name based on topic
            base_name := rec.ntfy_topic;
            try_name := base_name;
            suffix := 2;
            WHILE EXISTS (SELECT 1 FROM ntfy_configurations WHERE name = try_name) LOOP
                try_name := base_name || ' (' || suffix || ')';
                suffix := suffix + 1;
            END LOOP;

            INSERT INTO ntfy_configurations (name, url, topic, token)
            VALUES (try_name, rec.ntfy_url, rec.ntfy_topic, rec.ntfy_token)
            RETURNING id INTO config_id;
        END IF;

        -- Link projects to this configuration
        UPDATE projects
        SET ntfy_config_id = config_id
        WHERE ntfy_url = rec.ntfy_url
          AND ntfy_topic = rec.ntfy_topic
          AND COALESCE(ntfy_token, '') = COALESCE(rec.ntfy_token, '');
    END LOOP;
END $$;
