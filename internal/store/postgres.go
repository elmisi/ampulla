package store

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"time"

	"strings"

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
// Returns the issue and whether it's new or a regression (was resolved).
func (db *DB) UpsertIssue(ctx context.Context, projectID int64, fingerprint, title, level string, ts time.Time) (*event.UpsertResult, error) {
	// CTE captures old status before the upsert modifies it.
	// xmax = 0 in RETURNING means the row was INSERTed (new issue).
	row := db.pool.QueryRow(ctx, `
		WITH old AS (
			SELECT status FROM issues WHERE project_id = $1 AND fingerprint = $3
		)
		INSERT INTO issues (project_id, title, fingerprint, level, first_seen, last_seen, event_count)
		VALUES ($1, $2, $3, $4, $5, $5, 1)
		ON CONFLICT (project_id, fingerprint) DO UPDATE SET
			last_seen = GREATEST(issues.last_seen, EXCLUDED.last_seen),
			event_count = issues.event_count + 1,
			title = EXCLUDED.title,
			status = CASE WHEN issues.status = 'resolved' THEN 'unresolved' ELSE issues.status END
		RETURNING id, project_id, title, fingerprint, level, status, first_seen, last_seen, event_count,
			(xmax = 0) AS is_new,
			COALESCE((SELECT status FROM old), '') AS old_status
	`, projectID, title, fingerprint, level, ts)

	var issue event.Issue
	var isNew bool
	var oldStatus string
	err := row.Scan(
		&issue.ID, &issue.ProjectID, &issue.Title, &issue.Fingerprint,
		&issue.Level, &issue.Status, &issue.FirstSeen, &issue.LastSeen, &issue.EventCount,
		&isNew, &oldStatus,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert issue: %w", err)
	}

	return &event.UpsertResult{
		Issue:        &issue,
		IsNew:        isNew,
		IsRegression: !isNew && oldStatus == "resolved",
	}, nil
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
// Returns (0, nil) if the transaction already exists (duplicate event_id).
func (db *DB) InsertTransaction(ctx context.Context, t *event.Transaction) (int64, error) {
	var id int64
	err := db.pool.QueryRow(ctx, `
		INSERT INTO transactions (event_id, project_id, trace_id, span_id, op, name, duration_ms, status, timestamp, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (project_id, event_id) DO NOTHING
		RETURNING id
	`, t.EventID, t.ProjectID, t.TraceID, t.SpanID, t.Op, t.Name, t.DurationMs, t.Status, t.Timestamp, t.Data).Scan(&id)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return 0, nil // duplicate, skip
		}
		return 0, fmt.Errorf("insert transaction: %w", err)
	}
	return id, nil
}

// InsertSpans stores spans belonging to a transaction using a batch insert.
func (db *DB) InsertSpans(ctx context.Context, txnID int64, traceID uuid.UUID, spans []event.Span) error {
	if len(spans) == 0 {
		return nil
	}

	query := "INSERT INTO spans (transaction_id, trace_id, span_id, parent_span_id, op, description, duration_ms, status, timestamp, data) VALUES "
	args := make([]any, 0, len(spans)*10)
	for i, s := range spans {
		if i > 0 {
			query += ", "
		}
		base := i * 10
		query += fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10)
		args = append(args, txnID, traceID, s.SpanID, s.ParentSpanID, s.Op, s.Description, s.DurationMs, s.Status, s.Timestamp, s.Data)
	}

	_, err := db.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("insert spans: %w", err)
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
		SELECT p.id, p.org_id, p.name, p.slug, p.platform, p.created_at, p.ntfy_url, p.ntfy_topic, p.ntfy_token
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
		var platform, ntfyURL, ntfyTopic, ntfyToken *string
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &platform, &p.CreatedAt, &ntfyURL, &ntfyTopic, &ntfyToken); err != nil {
			return nil, err
		}
		if platform != nil {
			p.Platform = *platform
		}
		if ntfyURL != nil {
			p.NtfyURL = *ntfyURL
		}
		if ntfyTopic != nil {
			p.NtfyTopic = *ntfyTopic
		}
		if ntfyToken != nil {
			p.NtfyToken = *ntfyToken
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

// --- Admin CRUD Methods ---

// CreateOrganization creates a new organization.
func (db *DB) CreateOrganization(ctx context.Context, name, slug string) (*event.Organization, error) {
	var o event.Organization
	err := db.pool.QueryRow(ctx, `
		INSERT INTO organizations (name, slug) VALUES ($1, $2)
		RETURNING id, name, slug, created_at
	`, name, slug).Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create organization: %w", err)
	}
	return &o, nil
}

// UpdateOrganization updates an organization.
func (db *DB) UpdateOrganization(ctx context.Context, id int64, name, slug string) error {
	_, err := db.pool.Exec(ctx, `UPDATE organizations SET name = $1, slug = $2 WHERE id = $3`, name, slug, id)
	return err
}

// DeleteOrganization deletes an organization and cascading data.
func (db *DB) DeleteOrganization(ctx context.Context, id int64) error {
	_, err := db.pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, id)
	return err
}

// CreateProject creates a new project.
func (db *DB) CreateProject(ctx context.Context, orgID int64, name, slug, platform string) (*event.Project, error) {
	var p event.Project
	var plat *string
	if platform != "" {
		plat = &platform
	}
	err := db.pool.QueryRow(ctx, `
		INSERT INTO projects (org_id, name, slug, platform) VALUES ($1, $2, $3, $4)
		RETURNING id, org_id, name, slug, platform, created_at
	`, orgID, name, slug, plat).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &plat, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	if plat != nil {
		p.Platform = *plat
	}
	return &p, nil
}

// UpdateProject updates a project.
func (db *DB) UpdateProject(ctx context.Context, id int64, name, slug, platform, ntfyURL, ntfyTopic, ntfyToken string) error {
	var plat, nURL, nTopic, nToken *string
	if platform != "" {
		plat = &platform
	}
	if ntfyURL != "" {
		nURL = &ntfyURL
	}
	if ntfyTopic != "" {
		nTopic = &ntfyTopic
	}
	if ntfyToken != "" {
		nToken = &ntfyToken
	}
	_, err := db.pool.Exec(ctx, `UPDATE projects SET name = $1, slug = $2, platform = $3, ntfy_url = $4, ntfy_topic = $5, ntfy_token = $6 WHERE id = $7`,
		name, slug, plat, nURL, nTopic, nToken, id)
	return err
}

// DeleteProject deletes a project.
func (db *DB) DeleteProject(ctx context.Context, id int64) error {
	_, err := db.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
	return err
}

// ListAllProjects returns all projects.
func (db *DB) ListAllProjects(ctx context.Context) ([]event.Project, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT p.id, p.org_id, p.name, p.slug, p.platform, p.created_at, p.ntfy_url, p.ntfy_topic, p.ntfy_token
		FROM projects p ORDER BY p.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []event.Project
	for rows.Next() {
		var p event.Project
		var platform, ntfyURL, ntfyTopic, ntfyToken *string
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &platform, &p.CreatedAt, &ntfyURL, &ntfyTopic, &ntfyToken); err != nil {
			return nil, err
		}
		if platform != nil {
			p.Platform = *platform
		}
		if ntfyURL != nil {
			p.NtfyURL = *ntfyURL
		}
		if ntfyTopic != nil {
			p.NtfyTopic = *ntfyTopic
		}
		if ntfyToken != nil {
			p.NtfyToken = *ntfyToken
		}
		projects = append(projects, p)
	}
	return projects, nil
}

// ListProjectKeys returns DSN keys for a project.
func (db *DB) ListProjectKeys(ctx context.Context, projectID int64) ([]event.ProjectKey, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, project_id, public_key, secret_key, label, is_active
		FROM project_keys WHERE project_id = $1 ORDER BY id
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []event.ProjectKey
	for rows.Next() {
		var k event.ProjectKey
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.PublicKey, &k.SecretKey, &k.Label, &k.IsActive); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// CreateProjectKey creates a new DSN key pair.
func (db *DB) CreateProjectKey(ctx context.Context, projectID int64, label string) (*event.ProjectKey, error) {
	publicKey := generateKey()
	secretKey := generateKey()
	var k event.ProjectKey
	err := db.pool.QueryRow(ctx, `
		INSERT INTO project_keys (project_id, public_key, secret_key, label) VALUES ($1, $2, $3, $4)
		RETURNING id, project_id, public_key, secret_key, label, is_active
	`, projectID, publicKey, secretKey, label).Scan(&k.ID, &k.ProjectID, &k.PublicKey, &k.SecretKey, &k.Label, &k.IsActive)
	if err != nil {
		return nil, fmt.Errorf("create project key: %w", err)
	}
	return &k, nil
}

// ToggleProjectKey enables or disables a DSN key.
func (db *DB) ToggleProjectKey(ctx context.Context, id int64, isActive bool) error {
	_, err := db.pool.Exec(ctx, `UPDATE project_keys SET is_active = $1 WHERE id = $2`, isActive, id)
	return err
}

// DeleteProjectKey deletes a DSN key.
func (db *DB) DeleteProjectKey(ctx context.Context, id int64) error {
	_, err := db.pool.Exec(ctx, `DELETE FROM project_keys WHERE id = $1`, id)
	return err
}

// AdminListIssues returns issues, optionally filtered by project ID.
func (db *DB) AdminListIssues(ctx context.Context, projectID, cursor int64, limit int) ([]event.Issue, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	query := `SELECT id, project_id, title, fingerprint, level, status, first_seen, last_seen, event_count FROM issues`
	var args []any
	if projectID > 0 {
		query += ` WHERE project_id = $1 AND id > $2 ORDER BY last_seen DESC LIMIT $3`
		args = []any{projectID, cursor, limit}
	} else {
		query += ` WHERE id > $1 ORDER BY last_seen DESC LIMIT $2`
		args = []any{cursor, limit}
	}
	rows, err := db.pool.Query(ctx, query, args...)
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

// UpdateIssueStatus updates the status of an issue.
func (db *DB) UpdateIssueStatus(ctx context.Context, id int64, status string) error {
	_, err := db.pool.Exec(ctx, `UPDATE issues SET status = $1 WHERE id = $2`, status, id)
	return err
}

// DeleteIssue deletes an issue and its events.
func (db *DB) DeleteIssue(ctx context.Context, id int64) error {
	_, err := db.pool.Exec(ctx, `DELETE FROM issues WHERE id = $1`, id)
	return err
}

// AdminListTransactions returns transactions, optionally filtered by project ID.
func (db *DB) AdminListTransactions(ctx context.Context, projectID, cursor int64, limit int) ([]event.Transaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	var query string
	var args []any
	if projectID > 0 {
		query = `
			SELECT id, event_id, project_id, trace_id, span_id, op, name,
			       duration_ms, status, timestamp, data, received_at
			FROM transactions
			WHERE project_id = $1 AND id > $2
			ORDER BY timestamp DESC
			LIMIT $3`
		args = []any{projectID, cursor, limit}
	} else {
		query = `
			SELECT id, event_id, project_id, trace_id, span_id, op, name,
			       duration_ms, status, timestamp, data, received_at
			FROM transactions
			WHERE id > $1
			ORDER BY timestamp DESC
			LIMIT $2`
		args = []any{cursor, limit}
	}
	rows, err := db.pool.Query(ctx, query, args...)
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

// DashboardStats returns aggregate counts.
func (db *DB) DashboardStats(ctx context.Context) (map[string]int64, error) {
	stats := map[string]int64{}
	for _, q := range []struct {
		key   string
		query string
	}{
		{"organizations", "SELECT COUNT(*) FROM organizations"},
		{"projects", "SELECT COUNT(*) FROM projects"},
		{"issues", "SELECT COUNT(*) FROM issues"},
		{"events", "SELECT COUNT(*) FROM events"},
		{"transactions", "SELECT COUNT(*) FROM transactions"},
	} {
		var count int64
		if err := db.pool.QueryRow(ctx, q.query).Scan(&count); err != nil {
			return nil, err
		}
		stats[q.key] = count
	}
	return stats, nil
}

// GetPerformanceStats returns endpoint performance percentiles.
// If projectID > 0, filters by project. days controls the time window.
func (db *DB) GetPerformanceStats(ctx context.Context, projectID int64, days int) (*event.PerformanceStats, error) {
	stats := &event.PerformanceStats{}

	// Total count and oldest transaction
	if projectID > 0 {
		err := db.pool.QueryRow(ctx, `SELECT COUNT(*), MIN(timestamp) FROM transactions WHERE project_id = $1`, projectID).Scan(&stats.TotalCount, &stats.OldestTimestamp)
		if err != nil {
			return nil, fmt.Errorf("performance total: %w", err)
		}
	} else {
		err := db.pool.QueryRow(ctx, `SELECT COUNT(*), MIN(timestamp) FROM transactions`).Scan(&stats.TotalCount, &stats.OldestTimestamp)
		if err != nil {
			return nil, fmt.Errorf("performance total: %w", err)
		}
	}

	// Per-endpoint percentiles
	interval := fmt.Sprintf("%d days", days)
	var query string
	var args []any
	if projectID > 0 {
		query = `
			SELECT
				name, op, count(*) as cnt,
				round(avg(duration_ms)::numeric, 1),
				round(percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_ms)::numeric, 1),
				round(percentile_cont(0.75) WITHIN GROUP (ORDER BY duration_ms)::numeric, 1),
				round(percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms)::numeric, 1),
				round(percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms)::numeric, 1)
			FROM transactions
			WHERE timestamp > now() - $1::interval AND project_id = $2
			GROUP BY name, op
			ORDER BY cnt DESC
			LIMIT 20`
		args = []any{interval, projectID}
	} else {
		query = `
			SELECT
				name, op, count(*) as cnt,
				round(avg(duration_ms)::numeric, 1),
				round(percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_ms)::numeric, 1),
				round(percentile_cont(0.75) WITHIN GROUP (ORDER BY duration_ms)::numeric, 1),
				round(percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms)::numeric, 1),
				round(percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms)::numeric, 1)
			FROM transactions
			WHERE timestamp > now() - $1::interval
			GROUP BY name, op
			ORDER BY cnt DESC
			LIMIT 20`
		args = []any{interval}
	}

	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("performance endpoints: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var e event.EndpointStats
		if err := rows.Scan(&e.Name, &e.Op, &e.Count, &e.AvgMs, &e.P50, &e.P75, &e.P95, &e.P99); err != nil {
			return nil, err
		}
		stats.Endpoints = append(stats.Endpoints, e)
	}

	return stats, nil
}

// GetProjectNtfyConfig returns the ntfy notification config and name for a project.
func (db *DB) GetProjectNtfyConfig(ctx context.Context, projectID int64) (projectName, ntfyURL, ntfyTopic, ntfyToken string, err error) {
	var url, topic, token *string
	err = db.pool.QueryRow(ctx, `SELECT name, ntfy_url, ntfy_topic, ntfy_token FROM projects WHERE id = $1`, projectID).Scan(&projectName, &url, &topic, &token)
	if err != nil {
		return "", "", "", "", err
	}
	if url != nil {
		ntfyURL = *url
	}
	if topic != nil {
		ntfyTopic = *topic
	}
	if token != nil {
		ntfyToken = *token
	}
	return
}

// DeleteOldTransactions removes transactions (and their spans via CASCADE) older than before.
func (db *DB) DeleteOldTransactions(ctx context.Context, before time.Time) (int64, error) {
	tag, err := db.pool.Exec(ctx, `DELETE FROM transactions WHERE timestamp < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("delete old transactions: %w", err)
	}
	return tag.RowsAffected(), nil
}

func generateKey() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

// GetProjectByOrgAndSlug returns a project by org slug and project slug.
func (db *DB) GetProjectByOrgAndSlug(ctx context.Context, orgSlug, projectSlug string) (*event.Project, error) {
	row := db.pool.QueryRow(ctx, `
		SELECT p.id, p.org_id, p.name, p.slug, p.platform, p.created_at, p.ntfy_url, p.ntfy_topic, p.ntfy_token
		FROM projects p
		JOIN organizations o ON o.id = p.org_id
		WHERE o.slug = $1 AND p.slug = $2
	`, orgSlug, projectSlug)

	var p event.Project
	var platform, ntfyURL, ntfyTopic, ntfyToken *string
	err := row.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &platform, &p.CreatedAt, &ntfyURL, &ntfyTopic, &ntfyToken)
	if err != nil {
		return nil, err
	}
	if platform != nil {
		p.Platform = *platform
	}
	if ntfyURL != nil {
		p.NtfyURL = *ntfyURL
	}
	if ntfyTopic != nil {
		p.NtfyTopic = *ntfyTopic
	}
	if ntfyToken != nil {
		p.NtfyToken = *ntfyToken
	}
	return &p, nil
}
