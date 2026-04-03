ALTER TABLE agents ADD COLUMN IF NOT EXISTS execution_mode VARCHAR(32) NOT NULL DEFAULT 'server';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS local_runtime_kind VARCHAR(64);
ALTER TABLE agents ADD COLUMN IF NOT EXISTS bound_worker_id VARCHAR(255);

CREATE INDEX IF NOT EXISTS idx_agents_tenant_bound_worker
    ON agents(tenant_id, bound_worker_id)
    WHERE bound_worker_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS local_workers (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    tenant_id         UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    worker_id         VARCHAR(255) NOT NULL,
    runtime_kind      VARCHAR(64) NOT NULL,
    display_name      VARCHAR(255),
    status            VARCHAR(20) NOT NULL DEFAULT 'online',
    last_heartbeat_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, worker_id)
);

CREATE TABLE IF NOT EXISTS local_worker_jobs (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    worker_id    VARCHAR(255) NOT NULL,
    agent_id     UUID REFERENCES agents(id) ON DELETE SET NULL,
    task_id      UUID,
    job_type     VARCHAR(64) NOT NULL,
    status       VARCHAR(20) NOT NULL DEFAULT 'queued',
    payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
    result       JSONB,
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_local_workers_tenant_worker
    ON local_workers(tenant_id, worker_id);

CREATE INDEX IF NOT EXISTS idx_local_worker_jobs_tenant_worker_status
    ON local_worker_jobs(tenant_id, worker_id, status);

CREATE INDEX IF NOT EXISTS idx_local_worker_jobs_tenant_task
    ON local_worker_jobs(tenant_id, task_id)
    WHERE task_id IS NOT NULL;
