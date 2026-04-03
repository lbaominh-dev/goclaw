package localworker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const (
	JobTypeRunTask      = "run_task"
	EnvelopeJobDispatch = "job.dispatch"
)

// DispatchJobInput contains the minimal persisted context needed for local worker execution.
type DispatchJobInput struct {
	TenantID          uuid.UUID         `json:"tenantId"`
	WorkerID          string            `json:"workerId"`
	RuntimeKind       string            `json:"runtimeKind"`
	AgentID           uuid.UUID         `json:"agentId"`
	AgentKey          string            `json:"agentKey"`
	RunID             string            `json:"runId"`
	SessionKey        string            `json:"sessionKey"`
	Message           string            `json:"message"`
	UserID            string            `json:"userId,omitempty"`
	Channel           string            `json:"channel,omitempty"`
	ChannelType       string            `json:"channelType,omitempty"`
	ChatID            string            `json:"chatId,omitempty"`
	PeerKind          string            `json:"peerKind,omitempty"`
	TeamID            string            `json:"teamId,omitempty"`
	TeamTaskID        string            `json:"teamTaskId,omitempty"`
	ParentAgentID     string            `json:"parentAgentId,omitempty"`
	LeaderAgentID     string            `json:"leaderAgentId,omitempty"`
	LocalKey          string            `json:"localKey,omitempty"`
	WorkspaceChannel  string            `json:"workspaceChannel,omitempty"`
	WorkspaceChatID   string            `json:"workspaceChatId,omitempty"`
	TeamWorkspace     string            `json:"teamWorkspace,omitempty"`
	ExtraSystemPrompt string            `json:"extraSystemPrompt,omitempty"`
	RunContext        *store.RunContext `json:"runContext,omitempty"`
	TraceName         string            `json:"traceName,omitempty"`
	TraceTags         []string          `json:"traceTags,omitempty"`
	ModelOverride     string            `json:"modelOverride,omitempty"`
	LightContext      bool              `json:"lightContext,omitempty"`
	HideInput         bool              `json:"hideInput,omitempty"`
	ContentSuffix     string            `json:"contentSuffix,omitempty"`
	RunKind           string            `json:"runKind,omitempty"`
	ParentTraceID     string            `json:"parentTraceId,omitempty"`
	ParentRootSpanID  string            `json:"parentRootSpanId,omitempty"`
	LinkedTraceID     string            `json:"linkedTraceId,omitempty"`
	TaskID            *uuid.UUID        `json:"-"`
}

type Dispatcher struct {
	manager *Manager
	workers store.WorkerStore
}

func NewDispatcher(workers store.WorkerStore, manager *Manager) *Dispatcher {
	if workers == nil || manager == nil {
		return nil
	}
	return &Dispatcher{workers: workers, manager: manager}
}

func (d *Dispatcher) DispatchRun(ctx context.Context, input DispatchJobInput) (*store.WorkerJobData, error) {
	if d == nil || d.workers == nil || d.manager == nil {
		return nil, fmt.Errorf("local worker dispatcher not configured")
	}
	input.WorkerID = strings.TrimSpace(input.WorkerID)
	input.RuntimeKind = strings.TrimSpace(input.RuntimeKind)
	if input.WorkerID == "" {
		return nil, fmt.Errorf("local worker id is required")
	}
	if !d.manager.IsOnline(input.TenantID, input.WorkerID) {
		return nil, ErrWorkerNotConnected
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal local worker payload: %w", err)
	}
	now := time.Now().UTC()
	job := &store.WorkerJobData{
		BaseModel: store.BaseModel{
			ID:        store.GenNewID(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		TenantID: input.TenantID,
		WorkerID: input.WorkerID,
		AgentID:  nilIfNilUUID(input.AgentID),
		TaskID:   input.TaskID,
		JobType:  JobTypeRunTask,
		Status:   store.WorkerJobStatusQueued,
		Payload:  payload,
	}
	if err := d.workers.CreateJob(store.WithTenantID(ctx, input.TenantID), job); err != nil {
		return nil, err
	}
	if err := d.manager.Dispatch(ctx, input.TenantID, input.WorkerID, Envelope{
		Type: EnvelopeJobDispatch,
		Payload: map[string]any{
			"jobId":       job.ID.String(),
			"runtimeKind": input.RuntimeKind,
			"job":         json.RawMessage(payload),
		},
	}); err != nil {
		_ = d.workers.UpdateJobStatus(store.WithTenantID(ctx, input.TenantID), job.ID, store.WorkerJobStatusFailed, []byte(err.Error()))
		job.Status = store.WorkerJobStatusFailed
		job.Result = []byte(err.Error())
		return nil, err
	}
	return job, nil
}

func nilIfNilUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	copy := id
	return &copy
}
