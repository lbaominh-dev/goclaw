//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestSQLiteWorkerStore_RegisterAndCompleteJob(t *testing.T) {
	workerStore, ctx, db := newTestSQLiteWorkerStore(t)

	worker := &store.WorkerData{
		TenantID:    store.MasterTenantID,
		WorkerID:    "sqlite-worker-1",
		RuntimeKind: "wails_desktop",
		DisplayName: "SQLite Worker",
		Status:      store.WorkerStatusOnline,
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
	applyTestSQLiteWorkerAgentSchema(t, db)
	if _, err := db.ExecContext(ctx, `INSERT INTO agents (id, agent_key, owner_id, provider, model, tenant_id) VALUES (?, ?, ?, ?, ?, ?)`, agentID, "sqlite-worker-agent", "user-1", "openai", "gpt-4.1-mini", store.MasterTenantID); err != nil {
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

func TestSQLiteWorkerStore_MarkJobRunningMissingJob(t *testing.T) {
	workerStore, ctx, _ := newTestSQLiteWorkerStore(t)

	err := workerStore.MarkJobRunning(ctx, store.GenNewID())
	if err == nil {
		t.Fatal("expected missing job error, got nil")
	}
}

func TestSQLiteWorkerStore_MarkJobCompletedMissingJob(t *testing.T) {
	workerStore, ctx, _ := newTestSQLiteWorkerStore(t)

	err := workerStore.MarkJobCompleted(ctx, store.GenNewID(), []byte(`{"ok":true}`))
	if err == nil {
		t.Fatal("expected missing job error, got nil")
	}
}

func TestSQLiteWorkerStore_SchemaIncludesAgentFK(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "schema-upgrade.db"))
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stmts := []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL PRIMARY KEY)`,
		`INSERT INTO schema_version (version) VALUES (5)`,
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
			owner_id TEXT NOT NULL,
			provider TEXT NOT NULL DEFAULT 'openrouter',
			model TEXT NOT NULL,
			tenant_id TEXT NOT NULL REFERENCES tenants(id),
			created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE TABLE local_worker_jobs (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			worker_id TEXT NOT NULL,
			agent_id TEXT,
			task_id TEXT,
			job_type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			payload TEXT NOT NULL DEFAULT '{}',
			result TEXT,
			started_at TEXT,
			completed_at TEXT,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE INDEX idx_local_worker_jobs_tenant_worker_status ON local_worker_jobs(tenant_id, worker_id, status)`,
		`CREATE INDEX idx_local_worker_jobs_tenant_task ON local_worker_jobs(tenant_id, task_id) WHERE task_id IS NOT NULL`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup exec error for %q: %v", stmt, err)
		}
	}

	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema error: %v", err)
	}

	rows, err := db.Query(`PRAGMA foreign_key_list('local_worker_jobs')`)
	if err != nil {
		t.Fatalf("foreign key list error: %v", err)
	}
	defer rows.Close()

	var found bool
	for rows.Next() {
		var id, seq int
		var tableName, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &tableName, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatalf("foreign key scan error: %v", err)
		}
		if tableName == "agents" && from == "agent_id" && to == "id" && strings.EqualFold(onDelete, "SET NULL") {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("foreign key rows error: %v", err)
	}
	if !found {
		t.Fatal("expected local_worker_jobs.agent_id foreign key to agents(id) ON DELETE SET NULL")
	}

	assertSQLiteIndexOnTable(t, db, "idx_local_worker_jobs_tenant_worker_status", "local_worker_jobs")
	assertSQLiteIndexOnTable(t, db, "idx_local_worker_jobs_tenant_task", "local_worker_jobs")
}

func assertSQLiteIndexOnTable(t *testing.T, db *sql.DB, indexName, tableName string) {
	t.Helper()

	var actualTable string
	err := db.QueryRow(`SELECT tbl_name FROM sqlite_master WHERE type = 'index' AND name = ?`, indexName).Scan(&actualTable)
	if err != nil {
		t.Fatalf("index lookup error for %q: %v", indexName, err)
	}
	if actualTable != tableName {
		t.Fatalf("index %q attached to %q, want %q", indexName, actualTable, tableName)
	}
}

func newTestSQLiteWorkerStore(t *testing.T) (*SQLiteWorkerStore, context.Context, *sql.DB) {
	t.Helper()

	db, err := OpenDB(filepath.Join(t.TempDir(), "workers.db"))
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema error: %v", err)
	}
	applyTestSQLiteWorkerSchema(t, db)

	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	return NewSQLiteWorkerStore(db), ctx, db
}

func applyTestSQLiteWorkerSchema(t *testing.T, db *sql.DB) {
	t.Helper()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS local_workers (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			worker_id TEXT NOT NULL,
			runtime_kind TEXT NOT NULL,
			display_name TEXT,
			status TEXT NOT NULL DEFAULT 'online',
			last_heartbeat_at TEXT,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			UNIQUE(tenant_id, worker_id)
		)`,
		`CREATE TABLE IF NOT EXISTS local_worker_jobs (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			worker_id TEXT NOT NULL,
			agent_id TEXT,
			task_id TEXT,
			job_type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			payload TEXT NOT NULL DEFAULT '{}',
			result TEXT,
			started_at TEXT,
			completed_at TEXT,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_local_workers_tenant_worker ON local_workers(tenant_id, worker_id)`,
		`CREATE INDEX IF NOT EXISTS idx_local_worker_jobs_tenant_worker_status ON local_worker_jobs(tenant_id, worker_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_local_worker_jobs_tenant_task ON local_worker_jobs(tenant_id, task_id)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("schema exec error for %q: %v", stmt, err)
		}
	}

	if _, err := db.Exec(`DELETE FROM local_worker_jobs WHERE tenant_id = ? AND worker_id LIKE ?`, store.MasterTenantID, "sqlite-worker-%"); err != nil {
		t.Fatalf("cleanup jobs error: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM local_workers WHERE tenant_id = ? AND worker_id LIKE ?`, store.MasterTenantID, "sqlite-worker-%"); err != nil {
		t.Fatalf("cleanup workers error: %v", err)
	}
}

func applyTestSQLiteWorkerAgentSchema(t *testing.T, db *sql.DB) {
	t.Helper()

	stmt := `CREATE TABLE IF NOT EXISTS agents (
		id TEXT PRIMARY KEY,
		agent_key TEXT NOT NULL,
		owner_id TEXT NOT NULL,
		provider TEXT NOT NULL DEFAULT 'openrouter',
		model TEXT NOT NULL,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
		updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("agent schema exec error: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM agents WHERE id = ?`, uuid.Nil.String()); err != nil {
		t.Fatalf("agent schema verification error: %v", err)
	}
}
