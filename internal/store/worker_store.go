package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const (
	WorkerStatusOnline  = "online"
	WorkerStatusOffline = "offline"
)

const (
	WorkerJobStatusQueued    = "queued"
	WorkerJobStatusRunning   = "running"
	WorkerJobStatusCompleted = "completed"
	WorkerJobStatusFailed    = "failed"
)

// WorkerData represents a registered local worker endpoint.
type WorkerData struct {
	BaseModel
	TenantID        uuid.UUID  `json:"tenant_id"`
	WorkerID        string     `json:"worker_id"`
	RuntimeKind     string     `json:"runtime_kind"`
	DisplayName     string     `json:"display_name,omitempty"`
	Status          string     `json:"status"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty"`
}

// WorkerJobData represents a persisted job assigned to a local worker.
type WorkerJobData struct {
	BaseModel
	TenantID    uuid.UUID  `json:"tenant_id"`
	WorkerID    string     `json:"worker_id"`
	AgentID     *uuid.UUID `json:"agent_id,omitempty"`
	TaskID      *uuid.UUID `json:"task_id,omitempty"`
	JobType     string     `json:"job_type"`
	Status      string     `json:"status"`
	Payload     []byte     `json:"payload,omitempty"`
	Result      []byte     `json:"result,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// WorkerStore persists local worker registration and job lifecycle state.
type WorkerStore interface {
	Register(ctx context.Context, worker *WorkerData) error
	SetWorkerStatus(ctx context.Context, workerID, status string) error
	GetWorker(ctx context.Context, workerID string) (*WorkerData, error)
	CreateJob(ctx context.Context, job *WorkerJobData) error
	MarkJobRunning(ctx context.Context, jobID uuid.UUID) error
	MarkJobCompleted(ctx context.Context, jobID uuid.UUID, result []byte) error
	UpdateJobStatus(ctx context.Context, jobID uuid.UUID, status string, result []byte) error
	GetJob(ctx context.Context, jobID uuid.UUID) (*WorkerJobData, error)
}
