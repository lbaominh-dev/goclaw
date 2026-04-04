package pg

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

type PGWorkerEndpointStore struct {
	db *sql.DB
}

func NewPGWorkerEndpointStore(db *sql.DB) *PGWorkerEndpointStore {
	return &PGWorkerEndpointStore{db: db}
}

func (s *PGWorkerEndpointStore) Create(ctx context.Context, endpoint *store.WorkerEndpointData) error {
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

	return s.db.QueryRowContext(ctx, `
		INSERT INTO worker_endpoint_profiles (
			id, tenant_id, name, runtime_kind, endpoint_url, auth_token, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
		RETURNING created_at, updated_at
	`, endpoint.ID, tid, endpoint.Name, endpoint.RuntimeKind, endpoint.EndpointURL, endpoint.AuthToken, endpoint.CreatedAt, endpoint.UpdatedAt).
		Scan(&endpoint.CreatedAt, &endpoint.UpdatedAt)
}

func (s *PGWorkerEndpointStore) Get(ctx context.Context, id uuid.UUID) (*store.WorkerEndpointData, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, runtime_kind, endpoint_url, auth_token, created_at, updated_at
		FROM worker_endpoint_profiles
		WHERE id = $1 AND tenant_id = $2
	`, id, tid)
	return scanPGWorkerEndpoint(row)
}

func (s *PGWorkerEndpointStore) List(ctx context.Context) ([]store.WorkerEndpointData, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, runtime_kind, endpoint_url, auth_token, created_at, updated_at
		FROM worker_endpoint_profiles
		WHERE tenant_id = $1
		ORDER BY name ASC, created_at ASC
	`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]store.WorkerEndpointData, 0)
	for rows.Next() {
		endpoint, err := scanPGWorkerEndpoint(rows)
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

func (s *PGWorkerEndpointStore) Update(ctx context.Context, id uuid.UUID, updates map[string]any) error {
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

	args := make([]any, 0, len(filtered)+3)
	setClauses := make([]string, 0, len(filtered)+1)
	i := 1
	for _, key := range []string{"name", "runtime_kind", "endpoint_url", "auth_token"} {
		value, ok := filtered[key]
		if !ok {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, i))
		args = append(args, value)
		i++
	}
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", i))
	args = append(args, time.Now().UTC())
	q := fmt.Sprintf(`
		UPDATE worker_endpoint_profiles
		SET %s
		WHERE id = $%d AND tenant_id = $%d
	`, strings.Join(setClauses, ", "), i+1, i+2)
	args = append(args, id, tid)
	_, err = s.db.ExecContext(ctx, q, args...)
	return err
}

func (s *PGWorkerEndpointStore) Delete(ctx context.Context, id uuid.UUID) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		DELETE FROM worker_endpoint_profiles
		WHERE id = $1 AND tenant_id = $2
	`, id, tid)
	return err
}

func scanPGWorkerEndpoint(row interface{ Scan(...any) error }) (*store.WorkerEndpointData, error) {
	var endpoint store.WorkerEndpointData
	err := row.Scan(
		&endpoint.ID,
		&endpoint.TenantID,
		&endpoint.Name,
		&endpoint.RuntimeKind,
		&endpoint.EndpointURL,
		&endpoint.AuthToken,
		&endpoint.CreatedAt,
		&endpoint.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &endpoint, nil
}
