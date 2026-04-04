//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestSQLiteAgentStore_CreateAndGet_LocalWorkerFields(t *testing.T) {
	agentStore, ctx, db := newTestSQLiteAgentStore(t)
	endpointID := createSQLiteWorkerEndpoint(t, db, ctx, "sqlite-local-worker-endpoint")

	agent := &store.AgentData{
		TenantID:            store.MasterTenantID,
		AgentKey:            "sqlite-local-worker-agent",
		DisplayName:         "SQLite Local Worker",
		OwnerID:             "user-1",
		Provider:            "openai",
		Model:               "gpt-4.1-mini",
		ContextWindow:       32000,
		MaxToolIterations:   8,
		Workspace:           ".",
		RestrictToWorkspace: true,
		AgentType:           store.AgentTypeOpen,
		Status:              store.AgentStatusActive,
		ExecutionMode:       store.AgentExecutionModeLocalWorker,
		LocalRuntimeKind:    "wails_desktop",
		BoundWorkerID:       "worker-123",
	}
	setAgentWorkerEndpointID(t, agent, endpointID)

	if err := agentStore.Create(ctx, agent); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	got, err := agentStore.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}

	if got.ExecutionMode != store.AgentExecutionModeLocalWorker {
		t.Fatalf("ExecutionMode = %q, want %q", got.ExecutionMode, store.AgentExecutionModeLocalWorker)
	}
	if got.LocalRuntimeKind != "wails_desktop" {
		t.Fatalf("LocalRuntimeKind = %q, want wails_desktop", got.LocalRuntimeKind)
	}
	if got.BoundWorkerID != "worker-123" {
		t.Fatalf("BoundWorkerID = %q, want worker-123", got.BoundWorkerID)
	}
	if gotWorkerEndpointID := getAgentWorkerEndpointID(t, got); gotWorkerEndpointID != endpointID {
		t.Fatalf("WorkerEndpointID = %q, want %q", gotWorkerEndpointID, endpointID)
	}
}

func TestAgentExecutionSettingsRequireWorkerEndpointID(t *testing.T) {
	agentStore, ctx, db := newTestSQLiteAgentStore(t)
	endpointID := createSQLiteWorkerEndpoint(t, db, ctx, "sqlite-required-endpoint")

	agent := &store.AgentData{
		TenantID:            store.MasterTenantID,
		AgentKey:            "sqlite-update-local-worker-agent",
		DisplayName:         "SQLite Update Local Worker",
		OwnerID:             "user-1",
		Provider:            "openai",
		Model:               "gpt-4.1-mini",
		ContextWindow:       32000,
		MaxToolIterations:   8,
		Workspace:           ".",
		RestrictToWorkspace: true,
		AgentType:           store.AgentTypeOpen,
		Status:              store.AgentStatusActive,
	}
	if err := agentStore.Create(ctx, agent); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	if err := agentStore.Update(ctx, agent.ID, map[string]any{"execution_mode": store.AgentExecutionModeLocalWorker}); err == nil {
		t.Fatal("expected local_worker update without required fields to fail")
	}

	got, err := agentStore.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID after failed local_worker update error: %v", err)
	}
	if got.ExecutionMode != store.AgentExecutionModeServer {
		t.Fatalf("ExecutionMode after failed local_worker update = %q, want %q", got.ExecutionMode, store.AgentExecutionModeServer)
	}

	if err := agentStore.Update(ctx, agent.ID, map[string]any{
		"execution_mode":     store.AgentExecutionModeLocalWorker,
		"local_runtime_kind": "wails_desktop",
		"worker_endpoint_id": endpointID,
	}); err != nil {
		t.Fatalf("valid local_worker update error: %v", err)
	}

	got, err = agentStore.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID after valid local_worker update error: %v", err)
	}
	if gotWorkerEndpointID := getAgentWorkerEndpointID(t, got); gotWorkerEndpointID != endpointID {
		t.Fatalf("WorkerEndpointID after valid local_worker update = %q, want %q", gotWorkerEndpointID, endpointID)
	}

	if err := agentStore.Update(ctx, agent.ID, map[string]any{"execution_mode": store.AgentExecutionModeServer}); err == nil {
		t.Fatal("expected server update with stale local worker fields to fail")
	}

	got, err = agentStore.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID after failed server update error: %v", err)
	}
	if got.ExecutionMode != store.AgentExecutionModeLocalWorker || got.LocalRuntimeKind != "wails_desktop" || getAgentWorkerEndpointID(t, got) != endpointID {
		t.Fatalf("agent state changed after failed server update: %+v", got)
	}
}

func TestSQLiteAgentStore_UpdateTransitionsLocalWorkerToServer(t *testing.T) {
	agentStore, ctx, _ := newTestSQLiteAgentStore(t)
	db := getSQLiteAgentTestDB(t, agentStore)
	endpointID := createSQLiteWorkerEndpoint(t, db, ctx, "sqlite-transition-endpoint")

	agent := &store.AgentData{
		TenantID:            store.MasterTenantID,
		AgentKey:            "sqlite-transition-to-server-agent",
		DisplayName:         "SQLite Transition Agent",
		OwnerID:             "user-1",
		Provider:            "openai",
		Model:               "gpt-4.1-mini",
		ContextWindow:       32000,
		MaxToolIterations:   8,
		Workspace:           ".",
		RestrictToWorkspace: true,
		AgentType:           store.AgentTypeOpen,
		Status:              store.AgentStatusActive,
		ExecutionMode:       store.AgentExecutionModeLocalWorker,
		LocalRuntimeKind:    "wails_desktop",
		BoundWorkerID:       "worker-123",
	}
	setAgentWorkerEndpointID(t, agent, endpointID)
	if err := agentStore.Create(ctx, agent); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	if err := agentStore.Update(ctx, agent.ID, map[string]any{
		"execution_mode":     store.AgentExecutionModeServer,
		"local_runtime_kind": nil,
		"bound_worker_id":    nil,
		"worker_endpoint_id": nil,
	}); err != nil {
		t.Fatalf("Update error: %v", err)
	}

	got, err := agentStore.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if got.ExecutionMode != store.AgentExecutionModeServer {
		t.Fatalf("ExecutionMode = %q, want %q", got.ExecutionMode, store.AgentExecutionModeServer)
	}
	if got.LocalRuntimeKind != "" {
		t.Fatalf("LocalRuntimeKind = %q, want empty", got.LocalRuntimeKind)
	}
	if got.BoundWorkerID != "" {
		t.Fatalf("BoundWorkerID = %q, want empty", got.BoundWorkerID)
	}
	if gotWorkerEndpointID := getAgentWorkerEndpointID(t, got); gotWorkerEndpointID != "" {
		t.Fatalf("WorkerEndpointID = %q, want empty", gotWorkerEndpointID)
	}

	var localRuntimeKind, boundWorkerID, workerEndpointID sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT local_runtime_kind, bound_worker_id, worker_endpoint_id FROM agents WHERE id = ? AND tenant_id = ?`, agent.ID, store.MasterTenantID).Scan(&localRuntimeKind, &boundWorkerID, &workerEndpointID); err != nil {
		t.Fatalf("raw agent query error: %v", err)
	}
	if localRuntimeKind.Valid {
		t.Fatalf("local_runtime_kind persisted as %q, want NULL", localRuntimeKind.String)
	}
	if boundWorkerID.Valid {
		t.Fatalf("bound_worker_id persisted as %q, want NULL", boundWorkerID.String)
	}
	if workerEndpointID.Valid {
		t.Fatalf("worker_endpoint_id persisted as %q, want NULL", workerEndpointID.String)
	}
}

func TestSQLiteAgentStore_UpdateSkipsSoftDeletedAgent(t *testing.T) {
	agentStore, _, db := newTestSQLiteAgentStore(t)
	ctx := store.WithTenantID(context.Background(), store.MasterTenantID)

	agent := &store.AgentData{
		TenantID:            store.MasterTenantID,
		AgentKey:            "sqlite-soft-deleted-agent",
		DisplayName:         "SQLite Soft Deleted Agent",
		OwnerID:             "user-1",
		Provider:            "openai",
		Model:               "gpt-4.1-mini",
		ContextWindow:       32000,
		MaxToolIterations:   8,
		Workspace:           ".",
		RestrictToWorkspace: true,
		AgentType:           store.AgentTypeOpen,
		Status:              store.AgentStatusActive,
	}
	if err := agentStore.Create(ctx, agent); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	deletedAt := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `UPDATE agents SET deleted_at = ? WHERE id = ? AND tenant_id = ?`, deletedAt, agent.ID, store.MasterTenantID); err != nil {
		t.Fatalf("soft delete setup error: %v", err)
	}

	if err := agentStore.Update(ctx, agent.ID, map[string]any{"display_name": "Updated Name"}); err != nil {
		t.Fatalf("Update error: %v", err)
	}

	var displayName string
	if err := db.QueryRowContext(ctx, `SELECT display_name FROM agents WHERE id = ? AND tenant_id = ?`, agent.ID, store.MasterTenantID).Scan(&displayName); err != nil {
		t.Fatalf("raw agent query error: %v", err)
	}
	if displayName != "SQLite Soft Deleted Agent" {
		t.Fatalf("display_name = %q, want original value", displayName)
	}
}

func TestSQLiteAgentStore_CrossTenantUpdateSkipsSoftDeletedAgent(t *testing.T) {
	agentStore, ctx, db := newTestSQLiteAgentStore(t)

	agent := &store.AgentData{
		TenantID:            store.MasterTenantID,
		AgentKey:            "sqlite-cross-tenant-soft-deleted-agent",
		DisplayName:         "SQLite Cross Tenant Soft Deleted Agent",
		OwnerID:             "user-1",
		Provider:            "openai",
		Model:               "gpt-4.1-mini",
		ContextWindow:       32000,
		MaxToolIterations:   8,
		Workspace:           ".",
		RestrictToWorkspace: true,
		AgentType:           store.AgentTypeOpen,
		Status:              store.AgentStatusActive,
	}
	if err := agentStore.Create(ctx, agent); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	deletedAt := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `UPDATE agents SET deleted_at = ? WHERE id = ? AND tenant_id = ?`, deletedAt, agent.ID, store.MasterTenantID); err != nil {
		t.Fatalf("soft delete setup error: %v", err)
	}

	if err := agentStore.Update(ctx, agent.ID, map[string]any{"display_name": "Updated Cross Tenant Name"}); err != nil {
		t.Fatalf("Update error: %v", err)
	}

	var displayName string
	if err := db.QueryRowContext(ctx, `SELECT display_name FROM agents WHERE id = ? AND tenant_id = ?`, agent.ID, store.MasterTenantID).Scan(&displayName); err != nil {
		t.Fatalf("raw agent query error: %v", err)
	}
	if displayName != "SQLite Cross Tenant Soft Deleted Agent" {
		t.Fatalf("display_name = %q, want original value", displayName)
	}
}

func TestSQLiteAgentStore_UpdateExecutionSettingsSkipsSoftDeletedAgent(t *testing.T) {
	agentStore, _, db := newTestSQLiteAgentStore(t)
	ctx := store.WithTenantID(context.Background(), store.MasterTenantID)
	endpointID := createSQLiteWorkerEndpoint(t, db, ctx, "sqlite-soft-delete-endpoint")

	agent := &store.AgentData{
		TenantID:            store.MasterTenantID,
		AgentKey:            "sqlite-soft-deleted-execution-agent",
		DisplayName:         "SQLite Soft Deleted Execution Agent",
		OwnerID:             "user-1",
		Provider:            "openai",
		Model:               "gpt-4.1-mini",
		ContextWindow:       32000,
		MaxToolIterations:   8,
		Workspace:           ".",
		RestrictToWorkspace: true,
		AgentType:           store.AgentTypeOpen,
		Status:              store.AgentStatusActive,
		ExecutionMode:       store.AgentExecutionModeLocalWorker,
		LocalRuntimeKind:    "wails_desktop",
		BoundWorkerID:       "worker-123",
	}
	setAgentWorkerEndpointID(t, agent, endpointID)
	if err := agentStore.Create(ctx, agent); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	deletedAt := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `UPDATE agents SET deleted_at = ? WHERE id = ? AND tenant_id = ?`, deletedAt, agent.ID, store.MasterTenantID); err != nil {
		t.Fatalf("soft delete setup error: %v", err)
	}

	if err := agentStore.Update(ctx, agent.ID, map[string]any{
		"execution_mode":     store.AgentExecutionModeServer,
		"local_runtime_kind": nil,
		"bound_worker_id":    nil,
		"worker_endpoint_id": nil,
	}); err != nil {
		t.Fatalf("Update error: %v", err)
	}

	var executionMode string
	var localRuntimeKind, boundWorkerID, workerEndpointID sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT execution_mode, local_runtime_kind, bound_worker_id, worker_endpoint_id FROM agents WHERE id = ? AND tenant_id = ?`, agent.ID, store.MasterTenantID).Scan(&executionMode, &localRuntimeKind, &boundWorkerID, &workerEndpointID); err != nil {
		t.Fatalf("raw agent query error: %v", err)
	}
	if executionMode != store.AgentExecutionModeLocalWorker {
		t.Fatalf("execution_mode = %q, want original value", executionMode)
	}
	if !localRuntimeKind.Valid || localRuntimeKind.String != "wails_desktop" {
		t.Fatalf("local_runtime_kind = %+v, want original value", localRuntimeKind)
	}
	if !boundWorkerID.Valid || boundWorkerID.String != "worker-123" {
		t.Fatalf("bound_worker_id = %+v, want original value", boundWorkerID)
	}
	if !workerEndpointID.Valid || workerEndpointID.String != endpointID {
		t.Fatalf("worker_endpoint_id = %+v, want original value", workerEndpointID)
	}
}

func setAgentWorkerEndpointID(t *testing.T, agent *store.AgentData, endpointID string) {
	t.Helper()
	field := reflect.ValueOf(agent).Elem().FieldByName("WorkerEndpointID")
	if !field.IsValid() {
		t.Fatal("AgentData.WorkerEndpointID field missing")
	}
	field.SetString(endpointID)
}

func getAgentWorkerEndpointID(t *testing.T, agent *store.AgentData) string {
	t.Helper()
	field := reflect.ValueOf(agent).Elem().FieldByName("WorkerEndpointID")
	if !field.IsValid() {
		t.Fatal("AgentData.WorkerEndpointID field missing")
	}
	return field.String()
}

func createSQLiteWorkerEndpoint(t *testing.T, db *sql.DB, ctx context.Context, name string) string {
	t.Helper()
	endpointID := uuid.New().String()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO worker_endpoint_profiles (id, tenant_id, name, runtime_kind, endpoint_url, auth_token)
		VALUES (?, ?, ?, ?, ?, ?)
	`, endpointID, store.MasterTenantID, name, "wails_desktop", "http://127.0.0.1:18790", "token"); err != nil {
		t.Fatalf("insert worker endpoint error: %v", err)
	}
	return endpointID
}

func getSQLiteAgentTestDB(t *testing.T, store *SQLiteAgentStore) *sql.DB {
	t.Helper()
	if store == nil || store.db == nil {
		t.Fatal("SQLiteAgentStore test db is nil")
	}
	return store.db
}

func newTestSQLiteAgentStore(t *testing.T) (*SQLiteAgentStore, context.Context, *sql.DB) {
	t.Helper()

	db, err := OpenDB(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema error: %v", err)
	}
	applyTestSQLiteAgentSchema(t, db)

	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	return NewSQLiteAgentStore(db), ctx, db
}

func applyTestSQLiteAgentSchema(t *testing.T, db *sql.DB) {
	t.Helper()

	stmts := map[string]string{
		"execution_mode":     `ALTER TABLE agents ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'server'`,
		"local_runtime_kind": `ALTER TABLE agents ADD COLUMN local_runtime_kind TEXT`,
		"bound_worker_id":    `ALTER TABLE agents ADD COLUMN bound_worker_id TEXT`,
	}

	for col, stmt := range stmts {
		var exists int
		if err := db.QueryRow(`SELECT COUNT(1) FROM pragma_table_info('agents') WHERE name = ?`, col).Scan(&exists); err != nil {
			t.Fatalf("column lookup error for %q: %v", col, err)
		}
		if exists > 0 {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("schema exec error for %q: %v", stmt, err)
		}
	}
}
