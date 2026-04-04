DROP INDEX IF EXISTS idx_agents_tenant_workspace_key;

ALTER TABLE agents DROP COLUMN IF EXISTS workspace_key;
