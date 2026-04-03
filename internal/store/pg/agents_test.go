package pg

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestPGAgentStore_CreateAndGet_LocalWorkerFields(t *testing.T) {
	db := newTestPGAgentDB(t)
	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	agentStore := NewPGAgentStore(db)

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
	}

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
}

func TestPGAgentExecutionSettingsValidation(t *testing.T) {
	tests := []struct {
		name    string
		agent   store.AgentData
		wantErr bool
	}{
		{
			name:  "default server mode is valid",
			agent: store.AgentData{},
		},
		{
			name: "explicit server mode rejects local worker fields",
			agent: store.AgentData{
				ExecutionMode:    store.AgentExecutionModeServer,
				LocalRuntimeKind: "wails_desktop",
			},
			wantErr: true,
		},
		{
			name: "invalid execution mode rejected",
			agent: store.AgentData{
				ExecutionMode: "invalid",
			},
			wantErr: true,
		},
		{
			name: "local worker requires runtime kind and bound worker id",
			agent: store.AgentData{
				ExecutionMode: store.AgentExecutionModeLocalWorker,
			},
			wantErr: true,
		},
		{
			name: "local worker with required fields is valid",
			agent: store.AgentData{
				ExecutionMode:    store.AgentExecutionModeLocalWorker,
				LocalRuntimeKind: "wails_desktop",
				BoundWorkerID:    "worker-123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.ValidateAgentExecutionSettings(tt.agent.ExecutionMode, tt.agent.LocalRuntimeKind, tt.agent.BoundWorkerID)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestPGAgentStore_UpdateRejectsInvalidLocalWorkerSettings(t *testing.T) {
	db := newTestPGAgentDB(t)
	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	agentStore := NewPGAgentStore(db)

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
		"bound_worker_id":    "worker-123",
	}); err != nil {
		t.Fatalf("valid local_worker update error: %v", err)
	}

	if err := agentStore.Update(ctx, agent.ID, map[string]any{"execution_mode": store.AgentExecutionModeServer}); err == nil {
		t.Fatal("expected server update with stale local worker fields to fail")
	}

	got, err = agentStore.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID after failed server update error: %v", err)
	}
	if got.ExecutionMode != store.AgentExecutionModeLocalWorker || got.LocalRuntimeKind != "wails_desktop" || got.BoundWorkerID != "worker-123" {
		t.Fatalf("agent state changed after failed server update: %+v", got)
	}
}

func TestPGAgentStore_UpdateTransitionsLocalWorkerToServer(t *testing.T) {
	db := newTestPGAgentDB(t)
	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	agentStore := NewPGAgentStore(db)

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
	}
	if err := agentStore.Create(ctx, agent); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	if err := agentStore.Update(ctx, agent.ID, map[string]any{
		"execution_mode":     store.AgentExecutionModeServer,
		"local_runtime_kind": nil,
		"bound_worker_id":    nil,
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
