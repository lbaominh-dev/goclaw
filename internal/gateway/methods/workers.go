package methods

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/localworker"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

type WorkersMethods struct {
	workers  store.WorkerStore
	manager  *localworker.Manager
	teams    store.TeamStore
	events   bus.EventPublisher
	postTurn tools.PostTurnProcessor
}

func NewWorkersMethods(workers store.WorkerStore, manager *localworker.Manager, teams store.TeamStore, events bus.EventPublisher, postTurn tools.PostTurnProcessor) *WorkersMethods {
	return &WorkersMethods{workers: workers, manager: manager, teams: teams, events: events, postTurn: postTurn}
}

func (m *WorkersMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodWorkersRegister, m.handleRegister)
	router.Register(protocol.MethodWorkersJobStarted, m.handleJobStarted)
	router.Register(protocol.MethodWorkersJobOutput, m.handleJobOutput)
	router.Register(protocol.MethodWorkersJobStatus, m.handleJobStatus)
	router.Register(protocol.MethodWorkersJobCompleted, m.handleJobCompleted)
	router.Register(protocol.MethodWorkersJobFailed, m.handleJobFailed)
}

func (m *WorkersMethods) handleRegister(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params struct {
		WorkerID    string `json:"workerId"`
		RuntimeKind string `json:"runtimeKind"`
		DisplayName string `json:"displayName"`
	}
	if !decodeWorkerParams(req, client, &params) {
		return
	}
	params.WorkerID = strings.TrimSpace(params.WorkerID)
	params.RuntimeKind = strings.TrimSpace(params.RuntimeKind)
	if params.WorkerID == "" || params.RuntimeKind == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "workerId and runtimeKind are required"))
		return
	}
	if m.workers == nil || m.manager == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrFailedPrecondition, "worker gateway is not configured"))
		return
	}
	worker := &store.WorkerData{
		TenantID:    client.TenantID(),
		WorkerID:    params.WorkerID,
		RuntimeKind: params.RuntimeKind,
		DisplayName: params.DisplayName,
		Status:      store.WorkerStatusOnline,
	}
	registerCtx := store.WithTenantID(ctx, client.TenantID())
	if err := m.workers.Register(registerCtx, worker); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	if previousWorkerID := getClientRegisteredWorkerID(client); previousWorkerID != "" && previousWorkerID != params.WorkerID {
		if m.manager.DisconnectIfConnection(client.TenantID(), previousWorkerID, client) {
			if err := m.workers.SetWorkerStatus(registerCtx, previousWorkerID, store.WorkerStatusOffline); err != nil {
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
				return
			}
		}
	}
	if m.manager != nil {
		if m.manager.DisconnectIfConnection(client.TenantID(), params.WorkerID, client) && m.workers != nil {
			if err := m.workers.SetWorkerStatus(registerCtx, params.WorkerID, store.WorkerStatusOffline); err != nil {
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
				return
			}
		}
	}
	if _, err := m.manager.Register(client.TenantID(), params.WorkerID, client); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	setClientRegisteredWorkerID(client, params.WorkerID)
	if err := m.workers.SetWorkerStatus(registerCtx, params.WorkerID, store.WorkerStatusOnline); err != nil {
		m.manager.DisconnectIfConnection(client.TenantID(), params.WorkerID, client)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"workerId": params.WorkerID,
		"status":   store.WorkerStatusOnline,
	}))
}

func (m *WorkersMethods) handleJobStarted(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	jobID, _, ok := m.validateJobOwnership(ctx, client, req)
	if !ok {
		return
	}
	if err := m.handleWorkerJobStarted(store.WithTenantID(ctx, client.TenantID()), jobID); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"jobId": jobID.String(), "status": store.WorkerJobStatusRunning}))
}

func (m *WorkersMethods) handleJobOutput(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	jobID, job, ok := m.validateJobOwnership(ctx, client, req)
	if !ok {
		return
	}
	var params struct {
		JobID  string          `json:"jobId"`
		Output json.RawMessage `json:"output"`
	}
	if !decodeWorkerParams(req, client, &params) {
		return
	}
	if err := m.handleWorkerJobOutput(store.WithTenantID(ctx, client.TenantID()), job, params.Output); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"jobId": jobID.String(), "accepted": true}))
}

func (m *WorkersMethods) handleJobStatus(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	jobID, job, ok := m.validateJobOwnership(ctx, client, req)
	if !ok {
		return
	}
	var params struct {
		JobID  string          `json:"jobId"`
		Status string          `json:"status"`
		Detail json.RawMessage `json:"detail"`
	}
	if !decodeWorkerParams(req, client, &params) {
		return
	}
	status := strings.TrimSpace(params.Status)
	if status == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "status is required"))
		return
	}
	result := []byte(params.Detail)
	if len(result) == 0 {
		result = []byte("{}")
	}
	if err := m.handleWorkerJobStatus(store.WithTenantID(ctx, client.TenantID()), job, status, result); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"jobId": jobID.String(), "status": status}))
}

func (m *WorkersMethods) handleJobCompleted(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	jobID, job, ok := m.validateJobOwnership(ctx, client, req)
	if !ok {
		return
	}
	var params struct {
		JobID  string          `json:"jobId"`
		Result json.RawMessage `json:"result"`
	}
	if !decodeWorkerParams(req, client, &params) {
		return
	}
	result := []byte(params.Result)
	if len(result) == 0 {
		result = []byte("{}")
	}
	if err := m.handleWorkerJobCompleted(store.WithTenantID(ctx, client.TenantID()), job, result); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"jobId": jobID.String(), "status": store.WorkerJobStatusCompleted}))
}

func (m *WorkersMethods) handleJobFailed(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	jobID, job, ok := m.validateJobOwnership(ctx, client, req)
	if !ok {
		return
	}
	var params struct {
		JobID string          `json:"jobId"`
		Error json.RawMessage `json:"error"`
	}
	if !decodeWorkerParams(req, client, &params) {
		return
	}
	result := []byte(params.Error)
	if len(result) == 0 {
		result = []byte("{}")
	}
	if err := m.handleWorkerJobFailed(store.WithTenantID(ctx, client.TenantID()), job, result); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"jobId": jobID.String(), "status": store.WorkerJobStatusFailed}))
}

func (m *WorkersMethods) HandleOutboundWorkerMessage(ctx context.Context, endpointID uuid.UUID, reply localworker.WorkerReplyEnvelope) error {
	if m == nil || m.workers == nil {
		return fmt.Errorf("worker gateway is not configured")
	}
	jobID, err := uuid.Parse(strings.TrimSpace(reply.JobID))
	if err != nil {
		return fmt.Errorf("jobId must be a valid UUID: %w", err)
	}
	ctx = store.WithTenantID(ctx, store.TenantIDFromContext(ctx))
	job, err := m.workers.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("worker job not found: %s", jobID)
	}
	if job.TenantID != store.TenantIDFromContext(ctx) || job.WorkerID != endpointID.String() {
		return fmt.Errorf("worker endpoint %s does not own job %s", endpointID, jobID)
	}
	switch strings.TrimSpace(reply.Type) {
	case "job.started":
		return m.handleWorkerJobStarted(ctx, jobID)
	case "job.output":
		return m.handleWorkerJobOutput(ctx, job, reply.Payload)
	case "job.status":
		status := strings.TrimSpace(reply.Status)
		if status == "" {
			return fmt.Errorf("status is required")
		}
		result := []byte(reply.Payload)
		if len(result) == 0 {
			result = []byte("{}")
		}
		return m.handleWorkerJobStatus(ctx, job, status, result)
	case "job.completed":
		result := []byte(reply.Payload)
		if len(result) == 0 {
			result = []byte("{}")
		}
		return m.handleWorkerJobCompleted(ctx, job, result)
	case "job.failed":
		result := []byte(reply.Error)
		if len(result) == 0 {
			result = []byte("{}")
		}
		return m.handleWorkerJobFailed(ctx, job, result)
	default:
		return nil
	}
}

func (m *WorkersMethods) handleWorkerJobStarted(ctx context.Context, jobID uuid.UUID) error {
	return m.workers.MarkJobRunning(ctx, jobID)
}

func (m *WorkersMethods) handleWorkerJobOutput(ctx context.Context, job *store.WorkerJobData, output json.RawMessage) error {
	task, err := m.loadLinkedTaskByJob(ctx, job)
	if err != nil || task == nil {
		return err
	}
	step := describeWorkerPayload(output)
	m.broadcastLinkedTaskEvent(store.TenantIDFromContext(ctx), protocol.EventTeamTaskProgress, task,
		tools.BuildTaskEventPayload(
			task.TeamID.String(), task.ID.String(),
			store.TeamTaskStatusInProgress,
			"system", job.WorkerID,
			append(m.taskEventOptions(task), tools.WithProgress(0, step))...,
		),
	)
	return nil
}

func (m *WorkersMethods) handleWorkerJobStatus(ctx context.Context, job *store.WorkerJobData, status string, result []byte) error {
	if err := m.workers.UpdateJobStatus(ctx, job.ID, status, result); err != nil {
		return err
	}
	task, err := m.loadLinkedTaskByJob(ctx, job)
	if err != nil || task == nil {
		return err
	}
	percent := workerProgressPercent(result, task.ProgressPercent)
	step := workerProgressStep(status, result)
	if err := m.teams.UpdateTaskProgress(ctx, task.ID, task.TeamID, percent, step); err != nil {
		return err
	}
	m.broadcastLinkedTaskEvent(store.TenantIDFromContext(ctx), protocol.EventTeamTaskProgress, task,
		tools.BuildTaskEventPayload(
			task.TeamID.String(), task.ID.String(),
			store.TeamTaskStatusInProgress,
			"system", job.WorkerID,
			append(m.taskEventOptions(task), tools.WithProgress(percent, step))...,
		),
	)
	return nil
}

func (m *WorkersMethods) handleWorkerJobCompleted(ctx context.Context, job *store.WorkerJobData, result []byte) error {
	if err := m.workers.MarkJobCompleted(ctx, job.ID, result); err != nil {
		return err
	}
	task, err := m.loadLinkedTaskByJob(ctx, job)
	if err != nil || task == nil {
		return err
	}
	resultText := describeWorkerPayload(result)
	if err := m.teams.CompleteTask(ctx, task.ID, task.TeamID, resultText); err != nil {
		return err
	}
	m.broadcastLinkedTaskEvent(store.TenantIDFromContext(ctx), protocol.EventTeamTaskCompleted, task,
		tools.BuildTaskEventPayload(
			task.TeamID.String(), task.ID.String(),
			store.TeamTaskStatusCompleted,
			"system", job.WorkerID,
			m.taskEventOptions(task)...,
		),
	)
	m.dispatchLinkedTaskDependents(ctx, task.TeamID)
	return nil
}

func (m *WorkersMethods) handleWorkerJobFailed(ctx context.Context, job *store.WorkerJobData, result []byte) error {
	if err := m.workers.UpdateJobStatus(ctx, job.ID, store.WorkerJobStatusFailed, result); err != nil {
		return err
	}
	task, err := m.loadLinkedTaskByJob(ctx, job)
	if err != nil || task == nil {
		return err
	}
	reason := describeWorkerPayload(result)
	if err := m.teams.FailTask(ctx, task.ID, task.TeamID, reason); err != nil {
		return err
	}
	m.broadcastLinkedTaskEvent(store.TenantIDFromContext(ctx), protocol.EventTeamTaskFailed, task,
		tools.BuildTaskEventPayload(
			task.TeamID.String(), task.ID.String(),
			store.TeamTaskStatusFailed,
			"system", job.WorkerID,
			append(m.taskEventOptions(task), tools.WithReason(reason))...,
		),
	)
	m.dispatchLinkedTaskDependents(ctx, task.TeamID)
	return nil
}

func (m *WorkersMethods) loadLinkedTaskByJob(ctx context.Context, job *store.WorkerJobData) (*store.TeamTaskData, error) {
	if m == nil || m.teams == nil || job == nil || job.TaskID == nil {
		return nil, nil
	}
	task, err := m.teams.GetTask(ctx, *job.TaskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, nil
	}
	return task, nil
}

func (m *WorkersMethods) loadLinkedTask(ctx context.Context, client *gateway.Client, job *store.WorkerJobData) (*store.TeamTaskData, error) {
	return m.loadLinkedTaskByJob(store.WithTenantID(ctx, client.TenantID()), job)
}

func (m *WorkersMethods) broadcastLinkedTaskEvent(tenantID uuid.UUID, name string, task *store.TeamTaskData, payload protocol.TeamTaskEventPayload) {
	if m == nil || m.events == nil || task == nil {
		return
	}
	bus.BroadcastForTenant(m.events, name, tenantID, payload)
}

func (m *WorkersMethods) dispatchLinkedTaskDependents(ctx context.Context, teamID uuid.UUID) {
	if m == nil || m.postTurn == nil || teamID == uuid.Nil {
		return
	}
	m.postTurn.DispatchUnblockedTasks(ctx, teamID)
}

func (m *WorkersMethods) taskEventOptions(task *store.TeamTaskData) []tools.TaskEventOption {
	if task == nil {
		return nil
	}
	opts := []tools.TaskEventOption{
		tools.WithTaskInfo(task.TaskNumber, task.Subject),
		tools.WithUserID(task.UserID),
		tools.WithChannel(task.Channel),
		tools.WithChatID(task.ChatID),
	}
	if task.OwnerAgentKey != "" {
		opts = append(opts, tools.WithOwnerAgentKey(task.OwnerAgentKey))
	}
	if peerKind, ok := task.Metadata[tools.TaskMetaPeerKind].(string); ok && peerKind != "" {
		opts = append(opts, tools.WithPeerKind(peerKind))
	}
	return opts
}

func describeWorkerPayload(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "worker output"
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, key := range []string{"message", "delta", "text", "content", "line", "summary"} {
			if text := strings.TrimSpace(anyString(obj[key])); text != "" {
				return text
			}
		}
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "worker output"
	}
	return text
}

func workerProgressStep(status string, raw json.RawMessage) string {
	if text := describeWorkerPayload(raw); text != "worker output" {
		return text
	}
	status = strings.TrimSpace(status)
	if status == "" {
		return "worker status update"
	}
	return status
}

func workerProgressPercent(raw json.RawMessage, current int) int {
	percent := current
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, key := range []string{"progress", "percent"} {
			if value, ok := obj[key]; ok {
				if parsed, ok := anyInt(value); ok {
					percent = parsed
					break
				}
			}
		}
	}
	if percent < current {
		percent = current
	}
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

func anyString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		return ""
	}
}

func anyInt(v any) (int, bool) {
	switch val := v.(type) {
	case float64:
		return int(val), true
	case float32:
		return int(val), true
	case int:
		return val, true
	case int64:
		return int(val), true
	case int32:
		return int(val), true
	case json.Number:
		i, err := val.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func (m *WorkersMethods) validateJobOwnership(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) (uuid.UUID, *store.WorkerJobData, bool) {
	var params struct {
		JobID string `json:"jobId"`
	}
	if !decodeWorkerParams(req, client, &params) {
		return uuid.Nil, nil, false
	}
	jobID, err := uuid.Parse(strings.TrimSpace(params.JobID))
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "jobId must be a valid UUID"))
		return uuid.Nil, nil, false
	}
	registeredWorkerID := getClientRegisteredWorkerID(client)
	if registeredWorkerID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "worker is not registered on this connection"))
		return uuid.Nil, nil, false
	}
	if m.manager != nil {
		reg, ok := m.manager.Get(client.TenantID(), registeredWorkerID)
		if !ok || reg == nil || reg.Connection != client {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, fmt.Sprintf("worker %s is no longer active on this connection", registeredWorkerID)))
			return uuid.Nil, nil, false
		}
	}
	job, err := m.workers.GetJob(store.WithTenantID(ctx, client.TenantID()), jobID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return uuid.Nil, nil, false
	}
	if job == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, fmt.Sprintf("worker job not found: %s", jobID)))
		return uuid.Nil, nil, false
	}
	if job.TenantID != client.TenantID() || job.WorkerID != registeredWorkerID {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, fmt.Sprintf("worker %s does not own job %s", registeredWorkerID, jobID)))
		return uuid.Nil, nil, false
	}
	return jobID, job, true
}

func decodeWorkerParams(req *protocol.RequestFrame, client *gateway.Client, dst any) bool {
	if req.Params == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "params are required"))
		return false
	}
	if err := json.Unmarshal(req.Params, dst); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "malformed params: "+err.Error()))
		return false
	}
	return true
}

func getClientRegisteredWorkerID(client *gateway.Client) string {
	return client.RegisteredWorkerID()
}

func setClientRegisteredWorkerID(client *gateway.Client, workerID string) {
	client.SetRegisteredWorkerID(workerID)
}
