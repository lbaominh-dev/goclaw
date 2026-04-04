package pg

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestPGAgentStore_CreateAndGet_LocalWorkerFields(t *testing.T) {
	db := newTestPGAgentDB(t)
	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	agentStore := NewPGAgentStore(db)
	endpointID := createPGWorkerEndpoint(t, db, ctx, "pg-local-worker-endpoint")

	agent := &store.AgentData{
		TenantID:            store.MasterTenantID,
		AgentKey:            "pg-local-worker-agent",
		DisplayName:         "PG Local Worker",
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
		WorkspaceKey:        "desktop-main",
	}
	setPGAgentWorkerEndpointID(t, agent, endpointID)

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
	if gotWorkerEndpointID := getPGAgentWorkerEndpointID(t, got); gotWorkerEndpointID != endpointID {
		t.Fatalf("WorkerEndpointID = %q, want %q", gotWorkerEndpointID, endpointID)
	}
	if got.WorkspaceKey != "desktop-main" {
		t.Fatalf("WorkspaceKey = %q, want desktop-main", got.WorkspaceKey)
	}
}

func TestAgentExecutionSettingsRequireWorkerEndpointIDPG(t *testing.T) {
	db := newTestPGAgentDB(t)
	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	agentStore := NewPGAgentStore(db)
	endpointID := createPGWorkerEndpoint(t, db, ctx, "pg-required-endpoint")

	agent := &store.AgentData{
		TenantID:            store.MasterTenantID,
		AgentKey:            "pg-update-local-worker-agent",
		DisplayName:         "PG Update Local Worker",
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
		"workspace_key":      "desktop-main",
	}); err != nil {
		t.Fatalf("valid local_worker update error: %v", err)
	}

	got, err = agentStore.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID after valid local_worker update error: %v", err)
	}
	if gotWorkerEndpointID := getPGAgentWorkerEndpointID(t, got); gotWorkerEndpointID != endpointID {
		t.Fatalf("WorkerEndpointID after valid local_worker update = %q, want %q", gotWorkerEndpointID, endpointID)
	}
	if got.WorkspaceKey != "desktop-main" {
		t.Fatalf("WorkspaceKey after valid local_worker update = %q, want desktop-main", got.WorkspaceKey)
	}

	if err := agentStore.Update(ctx, agent.ID, map[string]any{"execution_mode": store.AgentExecutionModeServer}); err == nil {
		t.Fatal("expected server update with stale local worker fields to fail")
	}

	got, err = agentStore.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID after failed server update error: %v", err)
	}
	if got.ExecutionMode != store.AgentExecutionModeLocalWorker || got.LocalRuntimeKind != "wails_desktop" || getPGAgentWorkerEndpointID(t, got) != endpointID || got.WorkspaceKey != "desktop-main" {
		t.Fatalf("agent state changed after failed server update: %+v", got)
	}
}

func TestPGAgentStore_UpdateTransitionsLocalWorkerToServer(t *testing.T) {
	db := newTestPGAgentDB(t)
	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	agentStore := NewPGAgentStore(db)
	endpointID := createPGWorkerEndpoint(t, db, ctx, "pg-transition-endpoint")

	agent := &store.AgentData{
		TenantID:            store.MasterTenantID,
		AgentKey:            "pg-transition-to-server-agent",
		DisplayName:         "PG Transition Agent",
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
		WorkspaceKey:        "desktop-main",
	}
	setPGAgentWorkerEndpointID(t, agent, endpointID)
	if err := agentStore.Create(ctx, agent); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	if err := agentStore.Update(ctx, agent.ID, map[string]any{
		"execution_mode":     store.AgentExecutionModeServer,
		"local_runtime_kind": nil,
		"bound_worker_id":    nil,
		"worker_endpoint_id": nil,
		"workspace_key":      nil,
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
	if gotWorkerEndpointID := getPGAgentWorkerEndpointID(t, got); gotWorkerEndpointID != "" {
		t.Fatalf("WorkerEndpointID = %q, want empty", gotWorkerEndpointID)
	}
	if got.WorkspaceKey != "" {
		t.Fatalf("WorkspaceKey = %q, want empty", got.WorkspaceKey)
	}
}

func setPGAgentWorkerEndpointID(t *testing.T, agent *store.AgentData, endpointID string) {
	t.Helper()
	field := reflect.ValueOf(agent).Elem().FieldByName("WorkerEndpointID")
	if !field.IsValid() {
		t.Fatal("AgentData.WorkerEndpointID field missing")
	}
	field.SetString(endpointID)
}

func getPGAgentWorkerEndpointID(t *testing.T, agent *store.AgentData) string {
	t.Helper()
	field := reflect.ValueOf(agent).Elem().FieldByName("WorkerEndpointID")
	if !field.IsValid() {
		t.Fatal("AgentData.WorkerEndpointID field missing")
	}
	return field.String()
}

func createPGWorkerEndpoint(t *testing.T, db *sql.DB, ctx context.Context, name string) string {
	t.Helper()
	endpointID := uuid.New().String()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO worker_endpoint_profiles (id, tenant_id, name, runtime_kind, endpoint_url, auth_token)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, endpointID, store.MasterTenantID, name, "wails_desktop", "http://127.0.0.1:18790", "token"); err != nil {
		t.Fatalf("insert worker endpoint error: %v", err)
	}
	return endpointID
}

func newTestPGAgentDB(t *testing.T) *sql.DB {
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

	applyTestPGAgentSchema(t, db)
	return db
}

func applyTestPGAgentSchema(t *testing.T, db *sql.DB) {
	t.Helper()

	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`,
		`CREATE EXTENSION IF NOT EXISTS pgcrypto`,
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
		`CREATE TABLE IF NOT EXISTS agents (
			id UUID PRIMARY KEY,
			agent_key VARCHAR(100) NOT NULL,
			display_name VARCHAR(255),
			owner_id VARCHAR(255) NOT NULL,
			provider VARCHAR(50) NOT NULL DEFAULT 'openrouter',
			model VARCHAR(200) NOT NULL,
			context_window INT NOT NULL DEFAULT 200000,
			max_tool_iterations INT NOT NULL DEFAULT 20,
			workspace TEXT NOT NULL DEFAULT '.',
			restrict_to_workspace BOOLEAN NOT NULL DEFAULT true,
			tools_config JSONB NOT NULL DEFAULT '{}',
			sandbox_config JSONB,
			subagents_config JSONB,
			memory_config JSONB,
			compaction_config JSONB,
			context_pruning JSONB,
			other_config JSONB NOT NULL DEFAULT '{}',
			is_default BOOLEAN NOT NULL DEFAULT false,
			agent_type VARCHAR(20) NOT NULL DEFAULT 'open',
			status VARCHAR(20) DEFAULT 'active',
			frontmatter TEXT,
			budget_monthly_cents INTEGER,
			tenant_id UUID NOT NULL REFERENCES tenants(id),
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		)`,
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS execution_mode VARCHAR(32) NOT NULL DEFAULT 'server'`,
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS local_runtime_kind VARCHAR(64)`,
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS bound_worker_id VARCHAR(255)`,
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS worker_endpoint_id UUID REFERENCES worker_endpoint_profiles(id) ON DELETE SET NULL`,
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS workspace_key TEXT`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("schema exec error for %q: %v", stmt, err)
		}
	}

	if _, err := db.Exec(`DELETE FROM agents WHERE agent_key = $1`, "pg-local-worker-agent"); err != nil {
		t.Fatalf("cleanup agents error: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM agents WHERE agent_key = $1`, "pg-server-agent"); err != nil {
		t.Fatalf("cleanup agents error: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM agents WHERE tenant_id = $1 AND agent_key LIKE $2`, store.MasterTenantID, "pg-local-worker-agent%"); err != nil {
		t.Fatalf("cleanup pattern agents error: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM agents WHERE tenant_id = $1 AND agent_key LIKE $2`, store.MasterTenantID, "pg-server-agent%"); err != nil {
		t.Fatalf("cleanup pattern agents error: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM agents WHERE tenant_id = $1 AND agent_key LIKE $2`, store.MasterTenantID, "pg-update-local-worker-agent%"); err != nil {
		t.Fatalf("cleanup pattern agents error: %v", err)
	}
}
