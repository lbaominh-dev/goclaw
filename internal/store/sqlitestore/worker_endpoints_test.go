//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestSQLiteWorkerEndpointStore_CreateAndGet(t *testing.T) {
	endpointStore, ctx, _ := newTestSQLiteWorkerEndpointStore(t)

	endpoint := &store.WorkerEndpointData{
		TenantID:    store.MasterTenantID,
		Name:        "desktop-primary",
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

func TestSQLiteWorkerEndpointStore_SchemaIncludesProfilesAndAgentBinding(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "worker-endpoints-upgrade.db"))
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stmts := []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL PRIMARY KEY)`,
		`INSERT INTO schema_version (version) VALUES (6)`,
		`CREATE TABLE tenants (
			id TEXT NOT NULL PRIMARY KEY,
			name TEXT NOT NULL,
			slug TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			settings TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE TABLE agents (
			id TEXT NOT NULL PRIMARY KEY,
			agent_key TEXT NOT NULL,
			display_name TEXT,
			owner_id TEXT NOT NULL,
			provider TEXT NOT NULL DEFAULT 'openrouter',
			model TEXT NOT NULL,
			context_window INTEGER NOT NULL DEFAULT 200000,
			max_tool_iterations INTEGER NOT NULL DEFAULT 20,
			workspace TEXT NOT NULL DEFAULT '.',
			restrict_to_workspace BOOLEAN NOT NULL DEFAULT 1,
			tools_config TEXT NOT NULL DEFAULT '{}',
			sandbox_config TEXT,
			subagents_config TEXT,
			memory_config TEXT,
			compaction_config TEXT,
			context_pruning TEXT,
			other_config TEXT NOT NULL DEFAULT '{}',
			is_default BOOLEAN NOT NULL DEFAULT 0,
			agent_type TEXT NOT NULL DEFAULT 'open',
			status TEXT DEFAULT 'active',
			execution_mode TEXT NOT NULL DEFAULT 'server',
			local_runtime_kind TEXT,
			bound_worker_id TEXT,
			frontmatter TEXT,
			budget_monthly_cents INTEGER,
			tenant_id TEXT NOT NULL REFERENCES tenants(id),
			created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			deleted_at TEXT
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup exec error for %q: %v", stmt, err)
		}
	}

	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema error: %v", err)
	}

	assertSQLiteTableExists(t, db, "worker_endpoint_profiles")
	assertSQLiteColumnExists(t, db, "agents", "worker_endpoint_id")
	assertSQLiteIndexOnTable(t, db, "idx_agents_tenant_worker_endpoint", "agents")
	assertSQLiteIndexOnTable(t, db, "idx_worker_endpoint_profiles_tenant_name", "worker_endpoint_profiles")
	assertSQLiteForeignKeyOnDelete(t, db, "agents", "worker_endpoint_profiles", "worker_endpoint_id", "id", "SET NULL")
}

func assertSQLiteTableExists(t *testing.T, db *sql.DB, tableName string) {
	t.Helper()

	var actualName string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&actualName)
	if err != nil {
		t.Fatalf("table lookup error for %q: %v", tableName, err)
	}
	if actualName != tableName {
		t.Fatalf("table name = %q, want %q", actualName, tableName)
	}
}

func assertSQLiteColumnExists(t *testing.T, db *sql.DB, tableName, columnName string) {
	t.Helper()

	rows, err := db.Query(`PRAGMA table_info(` + tableName + `)`)
	if err != nil {
		t.Fatalf("table info error for %q: %v", tableName, err)
	}
	defer rows.Close()

	var found bool
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("table info scan error: %v", err)
		}
		if name == columnName {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table info rows error: %v", err)
	}
	if !found {
		t.Fatalf("expected column %q on table %q", columnName, tableName)
	}
}

func assertSQLiteForeignKeyOnDelete(t *testing.T, db *sql.DB, tableName, refTable, from, to, onDelete string) {
	t.Helper()

	rows, err := db.Query(`PRAGMA foreign_key_list(` + tableName + `)`)
	if err != nil {
		t.Fatalf("foreign key list error for %q: %v", tableName, err)
	}
	defer rows.Close()

	var found bool
	for rows.Next() {
		var id, seq int
		var tableNameFK, fromCol, toCol, onUpdate, onDeleteValue, match string
		if err := rows.Scan(&id, &seq, &tableNameFK, &fromCol, &toCol, &onUpdate, &onDeleteValue, &match); err != nil {
			t.Fatalf("foreign key scan error: %v", err)
		}
		if tableNameFK == refTable && fromCol == from && toCol == to && onDeleteValue == onDelete {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("foreign key rows error: %v", err)
	}
	if !found {
		t.Fatalf("expected foreign key %s(%s) -> %s(%s) ON DELETE %s", tableName, from, refTable, to, onDelete)
	}
}

func newTestSQLiteWorkerEndpointStore(t *testing.T) (*SQLiteWorkerEndpointStore, context.Context, *sql.DB) {
	t.Helper()

	db, err := OpenDB(filepath.Join(t.TempDir(), "worker-endpoints.db"))
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema error: %v", err)
	}

	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	return NewSQLiteWorkerEndpointStore(db), ctx, db
}

func TestSQLiteWorkerEndpointStore_GetMissing(t *testing.T) {
	endpointStore, ctx, _ := newTestSQLiteWorkerEndpointStore(t)

	gotEndpoint, err := endpointStore.Get(ctx, uuid.New())
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if gotEndpoint != nil {
		t.Fatalf("Get = %+v, want nil", gotEndpoint)
	}
}

func TestSQLiteWorkerEndpointStore_ListUpdateDelete(t *testing.T) {
	endpointStore, ctx, _ := newTestSQLiteWorkerEndpointStore(t)

	first := &store.WorkerEndpointData{
		TenantID:    store.MasterTenantID,
		Name:        "desktop-b",
		RuntimeKind: "wails_desktop",
		EndpointURL: "http://127.0.0.1:18791",
		AuthToken:   "token-b",
	}
	second := &store.WorkerEndpointData{
		TenantID:    store.MasterTenantID,
		Name:        "desktop-a",
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
	if items[0].Name != "desktop-a" || items[1].Name != "desktop-b" {
		t.Fatalf("List names = [%s %s], want [desktop-a desktop-b]", items[0].Name, items[1].Name)
	}

	if err := endpointStore.Update(ctx, second.ID, map[string]any{
		"name":         "desktop-aa",
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
	if updated.Name != "desktop-aa" {
		t.Fatalf("updated Name = %q, want %q", updated.Name, "desktop-aa")
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

func TestSQLiteWorkerEndpointStore_DeletingEndpointClearsAgentBinding(t *testing.T) {
	endpointStore, ctx, db := newTestSQLiteWorkerEndpointStore(t)

	endpoint := &store.WorkerEndpointData{
		TenantID:    store.MasterTenantID,
		Name:        "desktop-delete",
		RuntimeKind: "wails_desktop",
		EndpointURL: "http://127.0.0.1:18790",
		AuthToken:   "token-delete",
	}
	if err := endpointStore.Create(ctx, endpoint); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	agentID := store.GenNewID()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO agents (
			id, agent_key, owner_id, provider, model, execution_mode, tenant_id, worker_endpoint_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, agentID, "sqlite-endpoint-agent", "user-1", "openai", "gpt-4.1-mini", store.AgentExecutionModeServer, store.MasterTenantID, endpoint.ID); err != nil {
		t.Fatalf("insert agent error: %v", err)
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM worker_endpoint_profiles WHERE id = ? AND tenant_id = ?`, endpoint.ID, store.MasterTenantID); err != nil {
		t.Fatalf("delete endpoint error: %v", err)
	}

	var workerEndpointID sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT worker_endpoint_id FROM agents WHERE id = ? AND tenant_id = ?`, agentID, store.MasterTenantID).Scan(&workerEndpointID); err != nil {
		t.Fatalf("query agent binding error: %v", err)
	}
	if workerEndpointID.Valid {
		t.Fatalf("worker_endpoint_id = %q, want NULL after endpoint delete", workerEndpointID.String)
	}
}
