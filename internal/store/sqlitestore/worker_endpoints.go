//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type SQLiteWorkerEndpointStore struct {
	db *sql.DB
}

func NewSQLiteWorkerEndpointStore(db *sql.DB) *SQLiteWorkerEndpointStore {
	return &SQLiteWorkerEndpointStore{db: db}
}

func (s *SQLiteWorkerEndpointStore) Create(ctx context.Context, endpoint *store.WorkerEndpointData) error {
	if endpoint.ID == uuid.Nil {
		endpoint.ID = store.GenNewID()
	}
	now := time.Now().UTC()
	if endpoint.CreatedAt.IsZero() {
		endpoint.CreatedAt = now
	}
	endpoint.UpdatedAt = now
	tid := tenantIDForInsert(ctx)
	endpoint.TenantID = tid

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO worker_endpoint_profiles (
			id, tenant_id, name, runtime_kind, endpoint_url, auth_token, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, endpoint.ID, tid, endpoint.Name, endpoint.RuntimeKind, endpoint.EndpointURL, endpoint.AuthToken, endpoint.CreatedAt, endpoint.UpdatedAt)
	if err != nil {
		return err
	}

	stored, err := s.Get(store.WithTenantID(ctx, tid), endpoint.ID)
	if err != nil || stored == nil {
		return err
	}
	endpoint.CreatedAt = stored.CreatedAt
	endpoint.UpdatedAt = stored.UpdatedAt
	return nil
}

func (s *SQLiteWorkerEndpointStore) Get(ctx context.Context, id uuid.UUID) (*store.WorkerEndpointData, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, runtime_kind, endpoint_url, auth_token, created_at, updated_at
		FROM worker_endpoint_profiles
		WHERE id = ? AND tenant_id = ?
	`, id, tid)
	return scanSQLiteWorkerEndpoint(row)
}

func (s *SQLiteWorkerEndpointStore) List(ctx context.Context) ([]store.WorkerEndpointData, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, runtime_kind, endpoint_url, auth_token, created_at, updated_at
		FROM worker_endpoint_profiles
		WHERE tenant_id = ?
		ORDER BY name ASC, created_at ASC
	`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]store.WorkerEndpointData, 0)
	for rows.Next() {
		endpoint, err := scanSQLiteWorkerEndpoint(rows)
		if err != nil {
			return nil, err
		}
		if endpoint != nil {
			items = append(items, *endpoint)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *SQLiteWorkerEndpointStore) Update(ctx context.Context, id uuid.UUID, updates map[string]any) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}

	allowed := map[string]bool{
		"name":         true,
		"runtime_kind": true,
		"endpoint_url": true,
		"auth_token":   true,
	}
	filtered := make(map[string]any, len(updates))
	for key, value := range updates {
		if !allowed[key] {
			return fmt.Errorf("unsupported update field: %s", key)
		}
		filtered[key] = value
	}
	if len(filtered) == 0 {
		return nil
	}

	setClauses := make([]string, 0, len(filtered)+1)
	args := make([]any, 0, len(filtered)+3)
	for _, key := range []string{"name", "runtime_kind", "endpoint_url", "auth_token"} {
		value, ok := filtered[key]
		if !ok {
			continue
		}
		setClauses = append(setClauses, key+" = ?")
		args = append(args, value)
	}
	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, time.Now().UTC(), id, tid)
	_, err = s.db.ExecContext(ctx, `
		UPDATE worker_endpoint_profiles
		SET `+strings.Join(setClauses, ", ")+`
		WHERE id = ? AND tenant_id = ?
	`, args...)
	return err
}

func (s *SQLiteWorkerEndpointStore) Delete(ctx context.Context, id uuid.UUID) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		DELETE FROM worker_endpoint_profiles
		WHERE id = ? AND tenant_id = ?
	`, id, tid)
	return err
}

func scanSQLiteWorkerEndpoint(row interface{ Scan(...any) error }) (*store.WorkerEndpointData, error) {
	var endpoint store.WorkerEndpointData
	createdAt, updatedAt := scanTimePair()
	err := row.Scan(
		&endpoint.ID,
		&endpoint.TenantID,
		&endpoint.Name,
		&endpoint.RuntimeKind,
		&endpoint.EndpointURL,
		&endpoint.AuthToken,
		createdAt,
		updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	endpoint.CreatedAt = createdAt.Time
	endpoint.UpdatedAt = updatedAt.Time
	return &endpoint, nil
}
