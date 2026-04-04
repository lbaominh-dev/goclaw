ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS workspace_key TEXT;

CREATE INDEX IF NOT EXISTS idx_agents_tenant_workspace_key
    ON agents(tenant_id, workspace_key)
    WHERE workspace_key IS NOT NULL;
