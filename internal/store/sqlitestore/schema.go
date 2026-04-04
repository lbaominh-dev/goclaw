//go:build sqlite || sqliteonly

package sqlitestore

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"strings"
)

//go:embed schema.sql
var schemaSQL string

// SchemaVersion is the current SQLite schema version.
// Bump this when adding new migration steps below.
const SchemaVersion = 8

// migrations maps version → SQL to apply when upgrading FROM that version.
// schema.sql always represents the LATEST full schema (for fresh DBs).
// Existing DBs are patched incrementally via these steps.
//
// Example: to add a new column in the future:
//
//	var migrations = map[int]string{
//	    1: `ALTER TABLE agents ADD COLUMN new_col TEXT DEFAULT '';`,
//	}
//
// Then bump SchemaVersion to 2.
var migrations = map[int]string{
	// Version 1 → 2: add contact_type column to channel_contacts.
	1: `ALTER TABLE channel_contacts ADD COLUMN contact_type VARCHAR(20) NOT NULL DEFAULT 'user';`,
	// Version 2 → 3: promote cron payload fields to dedicated columns + add stateless flag.
	2: `ALTER TABLE cron_jobs ADD COLUMN stateless INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cron_jobs ADD COLUMN deliver INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cron_jobs ADD COLUMN deliver_channel TEXT NOT NULL DEFAULT '';
ALTER TABLE cron_jobs ADD COLUMN deliver_to TEXT NOT NULL DEFAULT '';
ALTER TABLE cron_jobs ADD COLUMN wake_heartbeat INTEGER NOT NULL DEFAULT 0;
UPDATE cron_jobs SET
  deliver = COALESCE(json_extract(payload, '$.deliver'), 0),
  deliver_channel = COALESCE(json_extract(payload, '$.channel'), ''),
  deliver_to = COALESCE(json_extract(payload, '$.to'), ''),
  wake_heartbeat = COALESCE(json_extract(payload, '$.wake_heartbeat'), 0)
WHERE payload IS NOT NULL;`,
	// Version 4 → 5: add thread_id, thread_type columns to channel_contacts for forum topic support.
	4: `ALTER TABLE channel_contacts ADD COLUMN thread_id VARCHAR(100);
ALTER TABLE channel_contacts ADD COLUMN thread_type VARCHAR(20);
DROP INDEX IF EXISTS idx_channel_contacts_tenant_type_sender;
CREATE UNIQUE INDEX idx_channel_contacts_tenant_type_sender
  ON channel_contacts(tenant_id, channel_type, sender_id, COALESCE(thread_id, ''));`,
	// Version 3 → 4: add subagent_tasks table for subagent lifecycle persistence.
	3: `CREATE TABLE IF NOT EXISTS subagent_tasks (
    id                TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    parent_agent_key  VARCHAR(255) NOT NULL,
    session_key       VARCHAR(500),
    subject           VARCHAR(255) NOT NULL,
    description       TEXT NOT NULL,
    status            VARCHAR(20) NOT NULL DEFAULT 'running',
    result            TEXT,
    depth             INTEGER NOT NULL DEFAULT 1,
    model             VARCHAR(255),
    provider          VARCHAR(255),
    iterations        INTEGER NOT NULL DEFAULT 0,
    input_tokens      INTEGER NOT NULL DEFAULT 0,
    output_tokens     INTEGER NOT NULL DEFAULT 0,
    origin_channel    VARCHAR(50),
    origin_chat_id    VARCHAR(255),
    origin_peer_kind  VARCHAR(20),
    origin_user_id    VARCHAR(255),
    spawned_by        TEXT,
    completed_at      TEXT,
    archived_at       TEXT,
    metadata          TEXT NOT NULL DEFAULT '{}',
    created_at        TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at        TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
 );
CREATE INDEX IF NOT EXISTS idx_subagent_tasks_parent_status ON subagent_tasks(tenant_id, parent_agent_key, status);
CREATE INDEX IF NOT EXISTS idx_subagent_tasks_session ON subagent_tasks(session_key);
CREATE INDEX IF NOT EXISTS idx_subagent_tasks_created ON subagent_tasks(tenant_id, created_at);`,
	6: `CREATE TABLE IF NOT EXISTS worker_endpoint_profiles (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    runtime_kind TEXT NOT NULL,
    endpoint_url TEXT NOT NULL,
    auth_token   TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(tenant_id, name)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_worker_endpoint_profiles_tenant_name ON worker_endpoint_profiles(tenant_id, name);
ALTER TABLE agents ADD COLUMN worker_endpoint_id TEXT;
CREATE INDEX IF NOT EXISTS idx_agents_tenant_worker_endpoint ON agents(tenant_id, worker_endpoint_id) WHERE worker_endpoint_id IS NOT NULL;`,
	7: `DROP INDEX IF EXISTS idx_agents_tenant_agent_key_active;
DROP INDEX IF EXISTS idx_agents_owner;
DROP INDEX IF EXISTS idx_agents_status;
DROP INDEX IF EXISTS idx_agents_tenant;
DROP INDEX IF EXISTS idx_agents_tenant_active;
DROP INDEX IF EXISTS idx_agents_tenant_bound_worker;
DROP INDEX IF EXISTS idx_agents_tenant_worker_endpoint;
ALTER TABLE agents RENAME TO agents__old;
CREATE TABLE agents (
    id                    TEXT NOT NULL PRIMARY KEY,
    agent_key             VARCHAR(100) NOT NULL,
    display_name          VARCHAR(255),
    owner_id              VARCHAR(255) NOT NULL,
    provider              VARCHAR(50) NOT NULL DEFAULT 'openrouter',
    model                 VARCHAR(200) NOT NULL,
    context_window        INT NOT NULL DEFAULT 200000,
    max_tool_iterations   INT NOT NULL DEFAULT 20,
    workspace             TEXT NOT NULL DEFAULT '.',
    restrict_to_workspace BOOLEAN NOT NULL DEFAULT 1,
    tools_config          TEXT NOT NULL DEFAULT '{}',
    sandbox_config        TEXT,
    subagents_config      TEXT,
    memory_config         TEXT,
    compaction_config     TEXT,
    context_pruning       TEXT,
    other_config          TEXT NOT NULL DEFAULT '{}',
    is_default            BOOLEAN NOT NULL DEFAULT 0,
    agent_type            VARCHAR(20) NOT NULL DEFAULT 'open',
    status                VARCHAR(20) DEFAULT 'active',
    execution_mode        VARCHAR(32) NOT NULL DEFAULT 'server',
    local_runtime_kind    TEXT,
    bound_worker_id       TEXT,
    worker_endpoint_id    TEXT REFERENCES worker_endpoint_profiles(id) ON DELETE SET NULL,
    frontmatter           TEXT,
    budget_monthly_cents  INTEGER,
    tenant_id             TEXT NOT NULL REFERENCES tenants(id),
    created_at            TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at            TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at            TEXT
);
INSERT INTO agents (
    id, agent_key, display_name, owner_id, provider, model, context_window, max_tool_iterations,
    workspace, restrict_to_workspace, tools_config, sandbox_config, subagents_config, memory_config,
    compaction_config, context_pruning, other_config, is_default, agent_type, status, execution_mode,
    local_runtime_kind, bound_worker_id, worker_endpoint_id, frontmatter, budget_monthly_cents,
    tenant_id, created_at, updated_at, deleted_at
)
SELECT
    id, agent_key, display_name, owner_id, provider, model, context_window, max_tool_iterations,
    workspace, restrict_to_workspace, tools_config, sandbox_config, subagents_config, memory_config,
    compaction_config, context_pruning, other_config, is_default, agent_type, status, execution_mode,
    local_runtime_kind, bound_worker_id, worker_endpoint_id, frontmatter, budget_monthly_cents,
    tenant_id, created_at, updated_at, deleted_at
FROM agents__old;
DROP TABLE agents__old;
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_tenant_agent_key_active ON agents(tenant_id, agent_key) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_agents_owner ON agents(owner_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_agents_tenant ON agents(tenant_id);
CREATE INDEX IF NOT EXISTS idx_agents_tenant_active ON agents(tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_agents_tenant_bound_worker ON agents(tenant_id, bound_worker_id) WHERE bound_worker_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_agents_tenant_worker_endpoint ON agents(tenant_id, worker_endpoint_id) WHERE worker_endpoint_id IS NOT NULL;`,
}

// EnsureSchema creates tables if they don't exist and applies incremental migrations.
//
// Flow:
//  1. Fresh DB (no schema_version row) → apply full schema.sql + set version = SchemaVersion
//  2. Existing DB with version < SchemaVersion → apply patches sequentially
//  3. Existing DB with version == SchemaVersion → no-op
//  4. Always: seed master tenant (idempotent)
func EnsureSchema(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER NOT NULL PRIMARY KEY
	)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	var current int
	err := db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&current)
	if err == sql.ErrNoRows {
		// Fresh database — apply full schema.
		slog.Info("sqlite: applying initial schema", "version", SchemaVersion)
		tx, txErr := db.Begin()
		if txErr != nil {
			return fmt.Errorf("begin schema tx: %w", txErr)
		}
		if _, err := tx.Exec(schemaSQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply schema: %w", err)
		}
		if _, err := tx.Exec("INSERT INTO schema_version (version) VALUES (?)", SchemaVersion); err != nil {
			tx.Rollback()
			return fmt.Errorf("set schema version: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit schema tx: %w", err)
		}
		return seedMasterTenant(db)
	}
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	// Apply incremental migrations for existing DBs.
	if current < SchemaVersion {
		slog.Info("sqlite: migrating schema", "from", current, "to", SchemaVersion)
		for v := current; v < SchemaVersion; v++ {
			tx, txErr := db.Begin()
			if txErr != nil {
				return fmt.Errorf("begin migration tx v%d: %w", v, txErr)
			}

			if v == 5 {
				if err := applyWorkerSchemaMigrationV5ToV6(tx); err != nil {
					tx.Rollback()
					return fmt.Errorf("apply migration v%d: %w", v, err)
				}
			} else {
				patch, ok := migrations[v]
				if !ok {
					tx.Rollback()
					return fmt.Errorf("sqlite: missing migration for version %d → %d", v, v+1)
				}
				if _, err := tx.Exec(patch); err != nil {
					tx.Rollback()
					return fmt.Errorf("apply migration v%d: %w", v, err)
				}
			}
			if _, err := tx.Exec(
				"UPDATE schema_version SET version = ? WHERE version = ?", v+1, v,
			); err != nil {
				tx.Rollback()
				return fmt.Errorf("update schema version v%d: %w", v, err)
			}
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("commit migration v%d: %w", v, err)
			}
			slog.Info("sqlite: applied migration", "version", v+1)
		}
	}

	return seedMasterTenant(db)
}

func applyWorkerSchemaMigrationV5ToV6(tx *sql.Tx) error {
	stmts := []string{
		`ALTER TABLE agents ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'server'`,
		`ALTER TABLE agents ADD COLUMN local_runtime_kind TEXT`,
		`ALTER TABLE agents ADD COLUMN bound_worker_id TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_agents_tenant_bound_worker ON agents(tenant_id, bound_worker_id) WHERE bound_worker_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS local_workers (
    id                TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    worker_id         TEXT NOT NULL,
    runtime_kind      TEXT NOT NULL,
    display_name      TEXT,
    status            TEXT NOT NULL DEFAULT 'online',
    last_heartbeat_at TEXT,
    created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(tenant_id, worker_id)
)`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}

	hasJobs, err := sqliteTableExists(tx, "local_worker_jobs")
	if err != nil {
		return err
	}

	if hasJobs {
		if _, err := tx.Exec(`ALTER TABLE local_worker_jobs RENAME TO local_worker_jobs__old`); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`CREATE TABLE local_worker_jobs (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    worker_id    TEXT NOT NULL,
    agent_id     TEXT REFERENCES agents(id) ON DELETE SET NULL,
    task_id      TEXT,
    job_type     TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'queued',
    payload      TEXT NOT NULL DEFAULT '{}',
    result       TEXT,
    started_at   TEXT,
    completed_at TEXT,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
)`); err != nil {
		return err
	}

	if hasJobs {
		if _, err := tx.Exec(`INSERT INTO local_worker_jobs (id, tenant_id, worker_id, agent_id, task_id, job_type, status, payload, result, started_at, completed_at, created_at, updated_at)
SELECT id, tenant_id, worker_id, agent_id, task_id, job_type, status, payload, result, started_at, completed_at, created_at, updated_at
FROM local_worker_jobs__old`); err != nil {
			return err
		}
		if _, err := tx.Exec(`DROP TABLE local_worker_jobs__old`); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_local_workers_tenant_worker ON local_workers(tenant_id, worker_id)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_local_worker_jobs_tenant_worker_status ON local_worker_jobs(tenant_id, worker_id, status)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_local_worker_jobs_tenant_task ON local_worker_jobs(tenant_id, task_id) WHERE task_id IS NOT NULL`); err != nil {
		return err
	}

	return nil
}

func sqliteTableExists(tx *sql.Tx, name string) (bool, error) {
	var found string
	err := tx.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.EqualFold(found, name), nil
}

// seedMasterTenant ensures the master tenant row exists (idempotent).
func seedMasterTenant(db *sql.DB) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO tenants (id, name, slug, status) VALUES (?, 'Master', 'master', 'active')`,
		"0193a5b0-7000-7000-8000-000000000001",
	)
	if err != nil {
		slog.Warn("sqlite: seed master tenant failed", "error", err)
	}
	return nil
}
