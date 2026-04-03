package pg

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestPGWorkerStore_RegisterAndCompleteJob(t *testing.T) {
	db := newTestPGWorkerDB(t)
	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	workerStore := NewPGWorkerStore(db)

	worker := &store.WorkerData{
		TenantID:        store.MasterTenantID,
		WorkerID:        "pg-worker-1",
		RuntimeKind:     "wails_desktop",
		DisplayName:     "PG Worker",
		Status:          store.WorkerStatusOnline,
		LastHeartbeatAt: nil,
	}
	if err := workerStore.Register(ctx, worker); err != nil {
		t.Fatalf("Register error: %v", err)
	}

	gotWorker, err := workerStore.GetWorker(ctx, worker.WorkerID)
	if err != nil {
		t.Fatalf("GetWorker error: %v", err)
	}
	if gotWorker == nil {
		t.Fatal("GetWorker returned nil worker")
	}
	if gotWorker.RuntimeKind != worker.RuntimeKind {
		t.Fatalf("RuntimeKind = %q, want %q", gotWorker.RuntimeKind, worker.RuntimeKind)
	}
	if gotWorker.DisplayName != worker.DisplayName {
		t.Fatalf("DisplayName = %q, want %q", gotWorker.DisplayName, worker.DisplayName)
	}
	if gotWorker.Status != store.WorkerStatusOnline {
		t.Fatalf("Status = %q, want %q", gotWorker.Status, store.WorkerStatusOnline)
	}

	agentID := store.GenNewID()
	taskID := store.GenNewID()
	applyTestPGWorkerAgentSchema(t, db)
	if _, err := db.Exec(`INSERT INTO agents (id, agent_key, owner_id, provider, model, tenant_id) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (id) DO NOTHING`, agentID, "pg-worker-agent", "user-1", "openai", "gpt-4.1-mini", store.MasterTenantID); err != nil {
		t.Fatalf("insert agent error: %v", err)
	}

	job := &store.WorkerJobData{
		TenantID: store.MasterTenantID,
		WorkerID: worker.WorkerID,
		AgentID:  &agentID,
		TaskID:   &taskID,
		JobType:  "run_task",
		Status:   store.WorkerJobStatusQueued,
		Payload:  []byte(`{"task":"sync"}`),
	}
	if err := workerStore.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob error: %v", err)
	}

	if err := workerStore.MarkJobRunning(ctx, job.ID); err != nil {
		t.Fatalf("MarkJobRunning error: %v", err)
	}

	result := []byte(`{"ok":true}`)
	if err := workerStore.MarkJobCompleted(ctx, job.ID, result); err != nil {
		t.Fatalf("MarkJobCompleted error: %v", err)
	}

	gotJob, err := workerStore.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob error: %v", err)
	}
	if gotJob == nil {
		t.Fatal("GetJob returned nil job")
	}
	if gotJob.Status != store.WorkerJobStatusCompleted {
		t.Fatalf("Status = %q, want %q", gotJob.Status, store.WorkerJobStatusCompleted)
	}
	if string(gotJob.Payload) != string(job.Payload) {
		t.Fatalf("Payload = %s, want %s", string(gotJob.Payload), string(job.Payload))
	}
	if string(gotJob.Result) != string(result) {
		t.Fatalf("Result = %s, want %s", string(gotJob.Result), string(result))
	}
	if gotJob.StartedAt == nil {
		t.Fatal("StartedAt = nil, want timestamp")
	}
	if gotJob.CompletedAt == nil {
		t.Fatal("CompletedAt = nil, want timestamp")
	}
	if gotJob.AgentID == nil || *gotJob.AgentID != agentID {
		t.Fatalf("AgentID = %v, want %v", gotJob.AgentID, agentID)
	}
	if gotJob.TaskID == nil || *gotJob.TaskID != taskID {
		t.Fatalf("TaskID = %v, want %v", gotJob.TaskID, taskID)
	}
}

func TestPGWorkerStore_MarkJobRunningMissingJob(t *testing.T) {
	db := newTestPGWorkerDB(t)
	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	workerStore := NewPGWorkerStore(db)

	err := workerStore.MarkJobRunning(ctx, store.GenNewID())
	if err == nil {
		t.Fatal("expected missing job error, got nil")
	}
}

func TestPGWorkerStore_MarkJobCompletedMissingJob(t *testing.T) {
	db := newTestPGWorkerDB(t)
	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	workerStore := NewPGWorkerStore(db)

	err := workerStore.MarkJobCompleted(ctx, store.GenNewID(), []byte(`{"ok":true}`))
	if err == nil {
		t.Fatal("expected missing job error, got nil")
	}
}

func newTestPGWorkerDB(t *testing.T) *sql.DB {
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

	applyTestPGWorkerSchema(t, db)
	return db
}

func applyTestPGWorkerSchema(t *testing.T, db *sql.DB) {
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
		`CREATE TABLE IF NOT EXISTS local_workers (
			id UUID PRIMARY KEY,
			tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			worker_id VARCHAR(255) NOT NULL,
			runtime_kind VARCHAR(64) NOT NULL,
			display_name VARCHAR(255),
			status VARCHAR(20) NOT NULL DEFAULT 'online',
			last_heartbeat_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (tenant_id, worker_id)
		)`,
		`CREATE TABLE IF NOT EXISTS local_worker_jobs (
			id UUID PRIMARY KEY,
			tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			worker_id VARCHAR(255) NOT NULL,
			agent_id UUID,
			task_id UUID,
			job_type VARCHAR(64) NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'queued',
			payload JSONB NOT NULL DEFAULT '{}'::jsonb,
			result JSONB,
			started_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_local_workers_tenant_worker ON local_workers(tenant_id, worker_id)`,
		`CREATE INDEX IF NOT EXISTS idx_local_worker_jobs_tenant_worker_status ON local_worker_jobs(tenant_id, worker_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_local_worker_jobs_tenant_task ON local_worker_jobs(tenant_id, task_id) WHERE task_id IS NOT NULL`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("schema exec error for %q: %v", stmt, err)
		}
	}

	if _, err := db.Exec(`DELETE FROM local_worker_jobs WHERE tenant_id = $1 AND worker_id LIKE $2`, store.MasterTenantID, "pg-worker-%"); err != nil {
		t.Fatalf("cleanup jobs error: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM local_workers WHERE tenant_id = $1 AND worker_id LIKE $2`, store.MasterTenantID, "pg-worker-%"); err != nil {
		t.Fatalf("cleanup workers error: %v", err)
	}
}

func applyTestPGWorkerAgentSchema(t *testing.T, db *sql.DB) {
	t.Helper()

	stmt := `CREATE TABLE IF NOT EXISTS agents (
		id UUID PRIMARY KEY,
		agent_key VARCHAR(100) NOT NULL,
		owner_id VARCHAR(255) NOT NULL,
		provider VARCHAR(50) NOT NULL DEFAULT 'openrouter',
		model VARCHAR(200) NOT NULL,
		tenant_id UUID NOT NULL REFERENCES tenants(id),
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW()
	)`
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("agent schema exec error: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM agents WHERE id = $1`, uuid.Nil); err != nil {
		// Keep a real query here so the helper exercises the table without depending on existing data.
		t.Fatalf("agent schema verification error: %v", err)
	}
}
