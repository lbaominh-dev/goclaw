//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type SQLiteWorkerStore struct {
	db *sql.DB
}

func NewSQLiteWorkerStore(db *sql.DB) *SQLiteWorkerStore {
	return &SQLiteWorkerStore{db: db}
}

func (s *SQLiteWorkerStore) Register(ctx context.Context, worker *store.WorkerData) error {
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

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO local_workers (id, tenant_id, worker_id, runtime_kind, display_name, status, last_heartbeat_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(tenant_id, worker_id) DO UPDATE SET
			runtime_kind = excluded.runtime_kind,
			display_name = excluded.display_name,
			status = excluded.status,
			last_heartbeat_at = excluded.last_heartbeat_at,
			updated_at = excluded.updated_at
	`,
		worker.ID, tid, worker.WorkerID, worker.RuntimeKind,
		sql.NullString{String: worker.DisplayName, Valid: worker.DisplayName != ""},
		worker.Status, nilTime(worker.LastHeartbeatAt), worker.CreatedAt, worker.UpdatedAt,
	)
	if err != nil {
		return err
	}
	stored, err := s.GetWorker(store.WithTenantID(ctx, tid), worker.WorkerID)
	if err != nil || stored == nil {
		return err
	}
	worker.ID = stored.ID
	worker.CreatedAt = stored.CreatedAt
	worker.UpdatedAt = stored.UpdatedAt
	worker.LastHeartbeatAt = stored.LastHeartbeatAt
	return nil
}

func (s *SQLiteWorkerStore) GetWorker(ctx context.Context, workerID string) (*store.WorkerData, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, worker_id, runtime_kind, display_name, status, last_heartbeat_at, created_at, updated_at
		FROM local_workers
		WHERE tenant_id = ? AND worker_id = ?
	`, tid, workerID)
	return scanSQLiteWorker(row)
}

func (s *SQLiteWorkerStore) SetWorkerStatus(ctx context.Context, workerID, status string) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE local_workers
		SET status = ?, updated_at = ?
		WHERE tenant_id = ? AND worker_id = ?
	`, status, time.Now().UTC(), tid, workerID)
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

func (s *SQLiteWorkerStore) CreateJob(ctx context.Context, job *store.WorkerJobData) error {
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
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		job.ID, tid, job.WorkerID, sqliteUUID(job.AgentID), sqliteUUID(job.TaskID), job.JobType, job.Status,
		string(job.Payload), nil, nilTime(job.StartedAt), nilTime(job.CompletedAt), job.CreatedAt, job.UpdatedAt,
	)
	return err
}

func (s *SQLiteWorkerStore) MarkJobRunning(ctx context.Context, jobID uuid.UUID) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE local_worker_jobs
		SET status = ?, started_at = COALESCE(started_at, ?), updated_at = ?
		WHERE id = ? AND tenant_id = ?
	`, store.WorkerJobStatusRunning, time.Now().UTC(), time.Now().UTC(), jobID, tid)
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

func (s *SQLiteWorkerStore) MarkJobCompleted(ctx context.Context, jobID uuid.UUID, result []byte) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	if len(result) == 0 {
		result = []byte("{}")
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE local_worker_jobs
		SET status = ?, result = ?, completed_at = ?, updated_at = ?
		WHERE id = ? AND tenant_id = ?
	`, store.WorkerJobStatusCompleted, string(result), time.Now().UTC(), time.Now().UTC(), jobID, tid)
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

func (s *SQLiteWorkerStore) UpdateJobStatus(ctx context.Context, jobID uuid.UUID, status string, result []byte) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	if len(result) == 0 {
		result = []byte("{}")
	}
	query := `
		UPDATE local_worker_jobs
		SET status = ?, result = ?, updated_at = ?`
	args := []any{status, string(result), time.Now().UTC()}
	if status == store.WorkerJobStatusFailed {
		query += `, completed_at = ?`
		args = append(args, time.Now().UTC())
	}
	query += ` WHERE id = ? AND tenant_id = ?`
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

func (s *SQLiteWorkerStore) GetJob(ctx context.Context, jobID uuid.UUID) (*store.WorkerJobData, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, worker_id, agent_id, task_id, job_type, status, payload, result,
			started_at, completed_at, created_at, updated_at
		FROM local_worker_jobs
		WHERE id = ? AND tenant_id = ?
	`, jobID, tid)
	return scanSQLiteWorkerJob(row)
}

func scanSQLiteWorker(row interface{ Scan(...any) error }) (*store.WorkerData, error) {
	var worker store.WorkerData
	var displayName sql.NullString
	var lastHeartbeat nullSqliteTime
	createdAt, updatedAt := scanTimePair()
	err := row.Scan(
		&worker.ID, &worker.TenantID, &worker.WorkerID, &worker.RuntimeKind, &displayName,
		&worker.Status, &lastHeartbeat, createdAt, updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	worker.DisplayName = displayName.String
	worker.CreatedAt = createdAt.Time
	worker.UpdatedAt = updatedAt.Time
	if lastHeartbeat.Valid {
		worker.LastHeartbeatAt = &lastHeartbeat.Time
	}
	return &worker, nil
}

func scanSQLiteWorkerJob(row interface{ Scan(...any) error }) (*store.WorkerJobData, error) {
	var job store.WorkerJobData
	var agentID, taskID sql.NullString
	var payload string
	var result sql.NullString
	var startedAt, completedAt nullSqliteTime
	createdAt, updatedAt := scanTimePair()
	err := row.Scan(
		&job.ID, &job.TenantID, &job.WorkerID, &agentID, &taskID, &job.JobType, &job.Status,
		&payload, &result, &startedAt, &completedAt, createdAt, updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	job.CreatedAt = createdAt.Time
	job.UpdatedAt = updatedAt.Time
	job.Payload = []byte(payload)
	if result.Valid {
		job.Result = []byte(result.String)
	}
	if agentID.Valid {
		u, err := uuid.Parse(agentID.String)
		if err != nil {
			return nil, err
		}
		job.AgentID = &u
	}
	if taskID.Valid {
		u, err := uuid.Parse(taskID.String)
		if err != nil {
			return nil, err
		}
		job.TaskID = &u
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}
	return &job, nil
}

func sqliteUUID(id *uuid.UUID) any {
	if id == nil || *id == uuid.Nil {
		return nil
	}
	return id.String()
}
