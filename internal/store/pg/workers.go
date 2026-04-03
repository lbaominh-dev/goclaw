package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type PGWorkerStore struct {
	db *sql.DB
}

func NewPGWorkerStore(db *sql.DB) *PGWorkerStore {
	return &PGWorkerStore{db: db}
}

func (s *PGWorkerStore) Register(ctx context.Context, worker *store.WorkerData) error {
	if worker.ID == uuid.Nil {
		worker.ID = store.GenNewID()
	}
	now := time.Now().UTC()
	if worker.CreatedAt.IsZero() {
		worker.CreatedAt = now
	}
	worker.UpdatedAt = now
	if worker.Status == "" {
		worker.Status = store.WorkerStatusOnline
	}
	tid := tenantIDForInsert(ctx)
	worker.TenantID = tid

	return s.db.QueryRowContext(ctx, `
		INSERT INTO local_workers (id, tenant_id, worker_id, runtime_kind, display_name, status, last_heartbeat_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (tenant_id, worker_id) DO UPDATE SET
			runtime_kind = EXCLUDED.runtime_kind,
			display_name = EXCLUDED.display_name,
			status = EXCLUDED.status,
			last_heartbeat_at = EXCLUDED.last_heartbeat_at,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at, updated_at
	`,
		worker.ID, tid, worker.WorkerID, worker.RuntimeKind,
		sql.NullString{String: worker.DisplayName, Valid: worker.DisplayName != ""},
		worker.Status, nilTime(worker.LastHeartbeatAt), worker.CreatedAt, worker.UpdatedAt,
	).Scan(&worker.ID, &worker.CreatedAt, &worker.UpdatedAt)
}

func (s *PGWorkerStore) GetWorker(ctx context.Context, workerID string) (*store.WorkerData, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, worker_id, runtime_kind, display_name, status, last_heartbeat_at, created_at, updated_at
		FROM local_workers
		WHERE tenant_id = $1 AND worker_id = $2
	`, tid, workerID)
	return scanPGWorker(row)
}

func (s *PGWorkerStore) SetWorkerStatus(ctx context.Context, workerID, status string) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE local_workers
		SET status = $1, updated_at = NOW()
		WHERE tenant_id = $2 AND worker_id = $3
	`, status, tid, workerID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("worker not found: %s", workerID)
	}
	return nil
}

func (s *PGWorkerStore) CreateJob(ctx context.Context, job *store.WorkerJobData) error {
	if job.ID == uuid.Nil {
		job.ID = store.GenNewID()
	}
	now := time.Now().UTC()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	job.UpdatedAt = now
	if job.Status == "" {
		job.Status = store.WorkerJobStatusQueued
	}
	if len(job.Payload) == 0 {
		job.Payload = []byte("{}")
	}
	tid := tenantIDForInsert(ctx)
	job.TenantID = tid

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO local_worker_jobs (
			id, tenant_id, worker_id, agent_id, task_id, job_type, status, payload, result,
			started_at, completed_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13
		)
	`,
		job.ID, tid, job.WorkerID, nilUUID(job.AgentID), nilUUID(job.TaskID), job.JobType, job.Status,
		job.Payload, nil, nilTime(job.StartedAt), nilTime(job.CompletedAt), job.CreatedAt, job.UpdatedAt,
	)
	return err
}

func (s *PGWorkerStore) MarkJobRunning(ctx context.Context, jobID uuid.UUID) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE local_worker_jobs
		SET status = $1, started_at = COALESCE(started_at, NOW()), updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3
	`, store.WorkerJobStatusRunning, jobID, tid)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("worker job not found: %s", jobID)
	}
	return nil
}

func (s *PGWorkerStore) MarkJobCompleted(ctx context.Context, jobID uuid.UUID, result []byte) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	if len(result) == 0 {
		result = []byte("{}")
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE local_worker_jobs
		SET status = $1, result = $2, completed_at = NOW(), updated_at = NOW()
		WHERE id = $3 AND tenant_id = $4
	`, store.WorkerJobStatusCompleted, result, jobID, tid)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("worker job not found: %s", jobID)
	}
	return nil
}

func (s *PGWorkerStore) UpdateJobStatus(ctx context.Context, jobID uuid.UUID, status string, result []byte) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	if len(result) == 0 {
		result = []byte("{}")
	}
	query := `
		UPDATE local_worker_jobs
		SET status = $1, result = $2, updated_at = NOW()`
	args := []any{status, result}
	if status == store.WorkerJobStatusFailed {
		query += `, completed_at = NOW()`
	}
	query += ` WHERE id = $3 AND tenant_id = $4`
	args = append(args, jobID, tid)
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("worker job not found: %s", jobID)
	}
	return nil
}

func (s *PGWorkerStore) GetJob(ctx context.Context, jobID uuid.UUID) (*store.WorkerJobData, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, worker_id, agent_id, task_id, job_type, status, payload, result,
			started_at, completed_at, created_at, updated_at
		FROM local_worker_jobs
		WHERE id = $1 AND tenant_id = $2
	`, jobID, tid)
	return scanPGWorkerJob(row)
}

func scanPGWorker(row interface{ Scan(...any) error }) (*store.WorkerData, error) {
	var worker store.WorkerData
	var displayName sql.NullString
	var lastHeartbeat sql.NullTime
	err := row.Scan(
		&worker.ID, &worker.TenantID, &worker.WorkerID, &worker.RuntimeKind, &displayName,
		&worker.Status, &lastHeartbeat, &worker.CreatedAt, &worker.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	worker.DisplayName = displayName.String
	if lastHeartbeat.Valid {
		worker.LastHeartbeatAt = &lastHeartbeat.Time
	}
	return &worker, nil
}

func scanPGWorkerJob(row interface{ Scan(...any) error }) (*store.WorkerJobData, error) {
	var job store.WorkerJobData
	var agentID, taskID uuid.NullUUID
	var result []byte
	var startedAt, completedAt sql.NullTime
	err := row.Scan(
		&job.ID, &job.TenantID, &job.WorkerID, &agentID, &taskID, &job.JobType, &job.Status,
		&job.Payload, &result, &startedAt, &completedAt, &job.CreatedAt, &job.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if agentID.Valid {
		job.AgentID = &agentID.UUID
	}
	if taskID.Valid {
		job.TaskID = &taskID.UUID
	}
	job.Result = result
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}
	return &job, nil
}
