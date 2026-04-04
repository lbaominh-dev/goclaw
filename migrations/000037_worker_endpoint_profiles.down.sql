DROP INDEX IF EXISTS idx_worker_endpoint_profiles_tenant_name;
DROP INDEX IF EXISTS idx_agents_tenant_worker_endpoint;

ALTER TABLE agents DROP COLUMN IF EXISTS worker_endpoint_id;

DROP TABLE IF EXISTS worker_endpoint_profiles;
