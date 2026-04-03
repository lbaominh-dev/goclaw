DROP INDEX IF EXISTS idx_agents_tenant_bound_worker;
DROP INDEX IF EXISTS idx_local_worker_jobs_tenant_task;
DROP INDEX IF EXISTS idx_local_worker_jobs_tenant_worker_status;
DROP INDEX IF EXISTS idx_local_workers_tenant_worker;

DROP TABLE IF EXISTS local_worker_jobs;
DROP TABLE IF EXISTS local_workers;

ALTER TABLE agents DROP COLUMN IF EXISTS bound_worker_id;
ALTER TABLE agents DROP COLUMN IF EXISTS local_runtime_kind;
ALTER TABLE agents DROP COLUMN IF EXISTS execution_mode;
