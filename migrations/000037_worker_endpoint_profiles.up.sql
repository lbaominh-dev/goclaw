CREATE TABLE IF NOT EXISTS worker_endpoint_profiles (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name         VARCHAR(255) NOT NULL,
    runtime_kind VARCHAR(64) NOT NULL,
    endpoint_url TEXT NOT NULL,
    auth_token   TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_worker_endpoint_profiles_tenant_name
    ON worker_endpoint_profiles(tenant_id, name);

ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS worker_endpoint_id UUID REFERENCES worker_endpoint_profiles(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_agents_tenant_worker_endpoint
    ON agents(tenant_id, worker_endpoint_id)
    WHERE worker_endpoint_id IS NOT NULL;
