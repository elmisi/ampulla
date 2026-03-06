package store

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"time"

	"github.com/elmisi/ampulla/internal/event"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations
var migrationsFS embed.FS

type DB struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	slog.Info("connected to database")
	return &DB{pool: pool}, nil
}

func (db *DB) Close() {
	db.pool.Close()
}

func (db *DB) RunMigrations(databaseURL string) error {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}
	slog.Info("migrations applied")
	return nil
}

// GetProjectByKey looks up a project by its DSN public key.
func (db *DB) GetProjectByKey(ctx context.Context, publicKey string) (*event.Project, *event.ProjectKey, error) {
	row := db.pool.QueryRow(ctx, `
		SELECT p.id, p.org_id, p.name, p.slug, p.platform, p.created_at,
		       k.id, k.project_id, k.public_key, k.secret_key, k.label, k.is_active
		FROM project_keys k
		JOIN projects p ON p.id = k.project_id
		WHERE k.public_key = $1
	`, publicKey)

	var proj event.Project
	var key event.ProjectKey
	var platform *string
	err := row.Scan(
		&proj.ID, &proj.OrgID, &proj.Name, &proj.Slug, &platform, &proj.CreatedAt,
		&key.ID, &key.ProjectID, &key.PublicKey, &key.SecretKey, &key.Label, &key.IsActive,
	)
	if err != nil {
		return nil, nil, err
	}
	if platform != nil {
		proj.Platform = *platform
	}
	return &proj, &key, nil
}

// UpsertIssue creates or updates an issue based on fingerprint.
func (db *DB) UpsertIssue(ctx context.Context, projectID int64, fingerprint, title, level string, ts time.Time) (*event.Issue, error) {
	row := db.pool.QueryRow(ctx, `
		INSERT INTO issues (project_id, title, fingerprint, level, first_seen, last_seen, event_count)
		VALUES ($1, $2, $3, $4, $5, $5, 1)
		ON CONFLICT (project_id, fingerprint) DO UPDATE SET
			last_seen = GREATEST(issues.last_seen, EXCLUDED.last_seen),
			event_count = issues.event_count + 1,
			title = EXCLUDED.title
		RETURNING id, project_id, title, fingerprint, level, status, first_seen, last_seen, event_count
	`, projectID, title, fingerprint, level, ts)

	var issue event.Issue
	err := row.Scan(
		&issue.ID, &issue.ProjectID, &issue.Title, &issue.Fingerprint,
		&issue.Level, &issue.Status, &issue.FirstSeen, &issue.LastSeen, &issue.EventCount,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert issue: %w", err)
	}
	return &issue, nil
}

// InsertEvent stores an error event.
func (db *DB) InsertEvent(ctx context.Context, e *event.Event) error {
	_, err := db.pool.Exec(ctx, `
		INSERT INTO events (event_id, project_id, issue_id, timestamp, platform, level, message, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (project_id, event_id) DO NOTHING
	`, e.EventID, e.ProjectID, e.IssueID, e.Timestamp, e.Platform, e.Level, e.Message, e.Data)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

// InsertTransaction stores a performance transaction and returns its database ID.
func (db *DB) InsertTransaction(ctx context.Context, t *event.Transaction) (int64, error) {
	var id int64
	err := db.pool.QueryRow(ctx, `
		INSERT INTO transactions (event_id, project_id, trace_id, span_id, op, name, duration_ms, status, timestamp, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (project_id, event_id) DO NOTHING
		RETURNING id
	`, t.EventID, t.ProjectID, t.TraceID, t.SpanID, t.Op, t.Name, t.DurationMs, t.Status, t.Timestamp, t.Data).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert transaction: %w", err)
	}
	return id, nil
}

// InsertSpans stores spans belonging to a transaction.
func (db *DB) InsertSpans(ctx context.Context, txnID int64, traceID uuid.UUID, spans []event.Span) error {
	for _, s := range spans {
		_, err := db.pool.Exec(ctx, `
			INSERT INTO spans (transaction_id, trace_id, span_id, parent_span_id, op, description, duration_ms, status, timestamp, data)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, txnID, traceID, s.SpanID, s.ParentSpanID, s.Op, s.Description, s.DurationMs, s.Status, s.Timestamp, s.Data)
		if err != nil {
			return fmt.Errorf("insert span: %w", err)
		}
	}
	return nil
}

// ListOrganizations returns all organizations.
func (db *DB) ListOrganizations(ctx context.Context) ([]event.Organization, error) {
	rows, err := db.pool.Query(ctx, `SELECT id, name, slug, created_at FROM organizations ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []event.Organization
	for rows.Next() {
		var o event.Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt); err != nil {
			return nil, err
		}
		orgs = append(orgs, o)
	}
	return orgs, nil
}

// ListProjects returns projects for an organization slug.
func (db *DB) ListProjects(ctx context.Context, orgSlug string) ([]event.Project, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT p.id, p.org_id, p.name, p.slug, p.platform, p.created_at
		FROM projects p
		JOIN organizations o ON o.id = p.org_id
		WHERE o.slug = $1
		ORDER BY p.name
	`, orgSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []event.Project
	for rows.Next() {
		var p event.Project
		var platform *string
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &platform, &p.CreatedAt); err != nil {
			return nil, err
		}
		if platform != nil {
			p.Platform = *platform
		}
		projects = append(projects, p)
	}
	return projects, nil
}

// ListIssues returns issues for a project, filtered by org and project slug.
func (db *DB) ListIssues(ctx context.Context, orgSlug, projectSlug string, cursor int64, limit int) ([]event.Issue, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := db.pool.Query(ctx, `
		SELECT i.id, i.project_id, i.title, i.fingerprint, i.level, i.status,
		       i.first_seen, i.last_seen, i.event_count
		FROM issues i
		JOIN projects p ON p.id = i.project_id
		JOIN organizations o ON o.id = p.org_id
		WHERE o.slug = $1 AND p.slug = $2 AND i.id > $3
		ORDER BY i.last_seen DESC
		LIMIT $4
	`, orgSlug, projectSlug, cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []event.Issue
	for rows.Next() {
		var i event.Issue
		if err := rows.Scan(&i.ID, &i.ProjectID, &i.Title, &i.Fingerprint, &i.Level,
			&i.Status, &i.FirstSeen, &i.LastSeen, &i.EventCount); err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	return issues, nil
}

// ListEventsByIssue returns events for an issue.
func (db *DB) ListEventsByIssue(ctx context.Context, issueID, cursor int64, limit int) ([]event.Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := db.pool.Query(ctx, `
		SELECT id, event_id, project_id, issue_id, timestamp, platform, level, message, data, received_at
		FROM events
		WHERE issue_id = $1 AND id > $2
		ORDER BY timestamp DESC
		LIMIT $3
	`, issueID, cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []event.Event
	for rows.Next() {
		var e event.Event
		var platform, level, message *string
		if err := rows.Scan(&e.ID, &e.EventID, &e.ProjectID, &e.IssueID, &e.Timestamp,
			&platform, &level, &message, &e.Data, &e.ReceivedAt); err != nil {
			return nil, err
		}
		if platform != nil {
			e.Platform = *platform
		}
		if level != nil {
			e.Level = *level
		}
		if message != nil {
			e.Message = *message
		}
		events = append(events, e)
	}
	return events, nil
}

// ListTransactions returns transactions for an organization.
func (db *DB) ListTransactions(ctx context.Context, orgSlug string, cursor int64, limit int) ([]event.Transaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := db.pool.Query(ctx, `
		SELECT t.id, t.event_id, t.project_id, t.trace_id, t.span_id, t.op, t.name,
		       t.duration_ms, t.status, t.timestamp, t.data, t.received_at
		FROM transactions t
		JOIN projects p ON p.id = t.project_id
		JOIN organizations o ON o.id = p.org_id
		WHERE o.slug = $1 AND t.id > $2
		ORDER BY t.timestamp DESC
		LIMIT $3
	`, orgSlug, cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txns []event.Transaction
	for rows.Next() {
		var t event.Transaction
		var op, status *string
		if err := rows.Scan(&t.ID, &t.EventID, &t.ProjectID, &t.TraceID, &t.SpanID,
			&op, &t.Name, &t.DurationMs, &status, &t.Timestamp, &t.Data, &t.ReceivedAt); err != nil {
			return nil, err
		}
		if op != nil {
			t.Op = *op
		}
		if status != nil {
			t.Status = *status
		}
		txns = append(txns, t)
	}
	return txns, nil
}

// GetProjectByOrgAndSlug returns a project by org slug and project slug.
func (db *DB) GetProjectByOrgAndSlug(ctx context.Context, orgSlug, projectSlug string) (*event.Project, error) {
	row := db.pool.QueryRow(ctx, `
		SELECT p.id, p.org_id, p.name, p.slug, p.platform, p.created_at
		FROM projects p
		JOIN organizations o ON o.id = p.org_id
		WHERE o.slug = $1 AND p.slug = $2
	`, orgSlug, projectSlug)

	var p event.Project
	var platform *string
	err := row.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &platform, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	if platform != nil {
		p.Platform = *platform
	}
	return &p, nil
}
