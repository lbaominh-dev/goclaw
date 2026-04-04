package pg

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestPGWorkerEndpointStore_CreateAndGet(t *testing.T) {
	db := newTestPGWorkerEndpointDB(t)
	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	endpointStore := NewPGWorkerEndpointStore(db)

	endpoint := &store.WorkerEndpointData{
		TenantID:    store.MasterTenantID,
		Name:        "pg-primary",
		RuntimeKind: "wails_desktop",
		EndpointURL: "http://127.0.0.1:18790",
		AuthToken:   "token-123",
	}
	if err := endpointStore.Create(ctx, endpoint); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	gotEndpoint, err := endpointStore.Get(ctx, endpoint.ID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if gotEndpoint == nil {
		t.Fatal("Get returned nil endpoint")
	}
	if gotEndpoint.TenantID != store.MasterTenantID {
		t.Fatalf("TenantID = %v, want %v", gotEndpoint.TenantID, store.MasterTenantID)
	}
	if gotEndpoint.Name != endpoint.Name {
		t.Fatalf("Name = %q, want %q", gotEndpoint.Name, endpoint.Name)
	}
	if gotEndpoint.RuntimeKind != endpoint.RuntimeKind {
		t.Fatalf("RuntimeKind = %q, want %q", gotEndpoint.RuntimeKind, endpoint.RuntimeKind)
	}
	if gotEndpoint.EndpointURL != endpoint.EndpointURL {
		t.Fatalf("EndpointURL = %q, want %q", gotEndpoint.EndpointURL, endpoint.EndpointURL)
	}
	if gotEndpoint.AuthToken != endpoint.AuthToken {
		t.Fatalf("AuthToken = %q, want %q", gotEndpoint.AuthToken, endpoint.AuthToken)
	}
	if gotEndpoint.CreatedAt.IsZero() {
		t.Fatal("CreatedAt = zero, want timestamp")
	}
	if gotEndpoint.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt = zero, want timestamp")
	}
}

func TestPGWorkerEndpointStore_GetMissing(t *testing.T) {
	db := newTestPGWorkerEndpointDB(t)
	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	endpointStore := NewPGWorkerEndpointStore(db)

	gotEndpoint, err := endpointStore.Get(ctx, uuid.New())
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if gotEndpoint != nil {
		t.Fatalf("Get = %+v, want nil", gotEndpoint)
	}
}

func TestPGWorkerEndpointStore_ListUpdateDelete(t *testing.T) {
	db := newTestPGWorkerEndpointDB(t)
	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	endpointStore := NewPGWorkerEndpointStore(db)

	first := &store.WorkerEndpointData{
		TenantID:    store.MasterTenantID,
		Name:        "pg-b",
		RuntimeKind: "wails_desktop",
		EndpointURL: "http://127.0.0.1:18791",
		AuthToken:   "token-b",
	}
	second := &store.WorkerEndpointData{
		TenantID:    store.MasterTenantID,
		Name:        "pg-a",
		RuntimeKind: "wails_desktop",
		EndpointURL: "http://127.0.0.1:18790",
		AuthToken:   "token-a",
	}
	if err := endpointStore.Create(ctx, first); err != nil {
		t.Fatalf("Create first error: %v", err)
	}
	if err := endpointStore.Create(ctx, second); err != nil {
		t.Fatalf("Create second error: %v", err)
	}

	items, err := endpointStore.List(ctx)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("List count = %d, want 2", len(items))
	}
	if items[0].Name != "pg-a" || items[1].Name != "pg-b" {
		t.Fatalf("List names = [%s %s], want [pg-a pg-b]", items[0].Name, items[1].Name)
	}

	if err := endpointStore.Update(ctx, second.ID, map[string]any{
		"name":         "pg-aa",
		"endpoint_url": "http://127.0.0.1:18792",
		"auth_token":   "token-aa",
	}); err != nil {
		t.Fatalf("Update error: %v", err)
	}

	updated, err := endpointStore.Get(ctx, second.ID)
	if err != nil {
		t.Fatalf("Get updated error: %v", err)
	}
	if updated == nil {
		t.Fatal("Get updated returned nil endpoint")
	}
	if updated.Name != "pg-aa" {
		t.Fatalf("updated Name = %q, want %q", updated.Name, "pg-aa")
	}
	if updated.EndpointURL != "http://127.0.0.1:18792" {
		t.Fatalf("updated EndpointURL = %q, want %q", updated.EndpointURL, "http://127.0.0.1:18792")
	}
	if updated.AuthToken != "token-aa" {
		t.Fatalf("updated AuthToken = %q, want %q", updated.AuthToken, "token-aa")
	}

	if err := endpointStore.Delete(ctx, first.ID); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	deleted, err := endpointStore.Get(ctx, first.ID)
	if err != nil {
		t.Fatalf("Get deleted error: %v", err)
	}
	if deleted != nil {
		t.Fatalf("Get deleted = %+v, want nil", deleted)
	}

	items, err = endpointStore.List(ctx)
	if err != nil {
		t.Fatalf("List after delete error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("List count after delete = %d, want 1", len(items))
	}
	if items[0].ID != second.ID {
		t.Fatalf("remaining endpoint ID = %v, want %v", items[0].ID, second.ID)
	}
}

func newTestPGWorkerEndpointDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("GOCLAW_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GOCLAW_POSTGRES_DSN is not set")
	}

	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	applyTestPGWorkerEndpointSchema(t, db)
	return db
}

func applyTestPGWorkerEndpointSchema(t *testing.T, db *sql.DB) {
	t.Helper()

	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`,
		`CREATE TABLE IF NOT EXISTS tenants (
			id UUID PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			slug VARCHAR(100) NOT NULL UNIQUE,
			status VARCHAR(20) NOT NULL DEFAULT 'active',
			settings JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`INSERT INTO tenants (id, name, slug, status)
		 VALUES ('0193a5b0-7000-7000-8000-000000000001', 'Master', 'master', 'active')
		 ON CONFLICT (id) DO NOTHING`,
		`CREATE TABLE IF NOT EXISTS worker_endpoint_profiles (
			id UUID PRIMARY KEY,
			tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			name VARCHAR(255) NOT NULL,
			runtime_kind VARCHAR(64) NOT NULL,
			endpoint_url TEXT NOT NULL,
			auth_token TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (tenant_id, name)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_worker_endpoint_profiles_tenant_name
			ON worker_endpoint_profiles(tenant_id, name)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("schema exec error for %q: %v", stmt, err)
		}
	}

	if _, err := db.Exec(`DELETE FROM worker_endpoint_profiles WHERE tenant_id = $1 AND name LIKE $2`, store.MasterTenantID, "pg-%"); err != nil {
		t.Fatalf("cleanup endpoints error: %v", err)
	}
}
