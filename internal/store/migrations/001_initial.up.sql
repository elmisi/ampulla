CREATE TABLE organizations (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id          BIGSERIAL PRIMARY KEY,
    org_id      BIGINT NOT NULL REFERENCES organizations(id),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL,
    platform    TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, slug)
);

CREATE TABLE project_keys (
    id          BIGSERIAL PRIMARY KEY,
    project_id  BIGINT NOT NULL REFERENCES projects(id),
    public_key  TEXT NOT NULL UNIQUE,
    secret_key  TEXT NOT NULL UNIQUE,
    label       TEXT NOT NULL DEFAULT 'Default',
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE issues (
    id          BIGSERIAL PRIMARY KEY,
    project_id  BIGINT NOT NULL REFERENCES projects(id),
    title       TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    level       TEXT NOT NULL DEFAULT 'error',
    status      TEXT NOT NULL DEFAULT 'unresolved',
    first_seen  TIMESTAMPTZ NOT NULL,
    last_seen   TIMESTAMPTZ NOT NULL,
    event_count BIGINT NOT NULL DEFAULT 1,
    UNIQUE(project_id, fingerprint)
);

CREATE TABLE events (
    id          BIGSERIAL PRIMARY KEY,
    event_id    UUID NOT NULL,
    project_id  BIGINT NOT NULL REFERENCES projects(id),
    issue_id    BIGINT NOT NULL REFERENCES issues(id),
    timestamp   TIMESTAMPTZ NOT NULL,
    platform    TEXT,
    level       TEXT,
    message     TEXT,
    data        JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id, event_id)
);

CREATE TABLE transactions (
    id          BIGSERIAL PRIMARY KEY,
    event_id    UUID NOT NULL,
    project_id  BIGINT NOT NULL REFERENCES projects(id),
    trace_id    UUID NOT NULL,
    span_id     TEXT NOT NULL,
    op          TEXT,
    name        TEXT NOT NULL,
    duration_ms DOUBLE PRECISION,
    status      TEXT,
    timestamp   TIMESTAMPTZ NOT NULL,
    data        JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id, event_id)
);

CREATE TABLE spans (
    id              BIGSERIAL PRIMARY KEY,
    transaction_id  BIGINT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    trace_id        UUID NOT NULL,
    span_id         TEXT NOT NULL,
    parent_span_id  TEXT,
    op              TEXT,
    description     TEXT,
    duration_ms     DOUBLE PRECISION,
    status          TEXT,
    timestamp       TIMESTAMPTZ NOT NULL,
    data            JSONB
);

CREATE INDEX idx_events_project_timestamp ON events(project_id, timestamp DESC);
CREATE INDEX idx_events_issue ON events(issue_id, timestamp DESC);
CREATE INDEX idx_issues_project_status ON issues(project_id, status);
CREATE INDEX idx_transactions_project_timestamp ON transactions(project_id, timestamp DESC);
CREATE INDEX idx_transactions_trace ON transactions(trace_id);
CREATE INDEX idx_spans_transaction ON spans(transaction_id);
CREATE INDEX idx_spans_trace ON spans(trace_id);
