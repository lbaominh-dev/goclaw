package methods

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/localworker"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

type workerStoreStub struct {
	registered       []*store.WorkerData
	jobs             map[uuid.UUID]*store.WorkerJobData
	runningJobs      []uuid.UUID
	completed        map[uuid.UUID][]byte
	workerStatuses   map[string]string
	jobStatusUpdates map[uuid.UUID]jobStatusUpdate
}

type jobStatusUpdate struct {
	status string
	result []byte
}

type workerTeamStoreStub struct {
	task          *store.TeamTaskData
	completeCalls []teamTaskTerminalCall
	failCalls     []teamTaskTerminalCall
	progressCalls []teamTaskProgressCall
}

type teamTaskTerminalCall struct {
	taskID uuid.UUID
	teamID uuid.UUID
	text   string
}

type teamTaskProgressCall struct {
	taskID  uuid.UUID
	teamID  uuid.UUID
	percent int
	step    string
}

type recordedBusEvent struct {
	name     string
	tenantID uuid.UUID
	payload  any
}

type recordingEventPublisher struct {
	events []recordedBusEvent
}

type workerPostTurnStub struct {
	dispatchedTeamIDs []uuid.UUID
}

func (p *recordingEventPublisher) Subscribe(string, bus.EventHandler) {}
func (p *recordingEventPublisher) Unsubscribe(string)                 {}
func (p *recordingEventPublisher) Broadcast(event bus.Event) {
	p.events = append(p.events, recordedBusEvent{name: event.Name, tenantID: event.TenantID, payload: event.Payload})
}

func (p *workerPostTurnStub) ProcessPendingTasks(context.Context, uuid.UUID, []uuid.UUID) error {
	return nil
}
func (p *workerPostTurnStub) DispatchUnblockedTasks(_ context.Context, teamID uuid.UUID) {
	p.dispatchedTeamIDs = append(p.dispatchedTeamIDs, teamID)
}

func (s *workerStoreStub) Register(_ context.Context, worker *store.WorkerData) error {
	copy := *worker
	s.registered = append(s.registered, &copy)
	if s.workerStatuses == nil {
		s.workerStatuses = make(map[string]string)
	}
	s.workerStatuses[worker.WorkerID] = worker.Status
	return nil
}

func (s *workerStoreStub) GetWorker(_ context.Context, workerID string) (*store.WorkerData, error) {
	for i := len(s.registered) - 1; i >= 0; i-- {
		if s.registered[i].WorkerID == workerID {
			copy := *s.registered[i]
			return &copy, nil
		}
	}
	return nil, nil
}

func (s *workerStoreStub) CreateJob(_ context.Context, _ *store.WorkerJobData) error { return nil }

func (s *workerStoreStub) MarkJobRunning(_ context.Context, jobID uuid.UUID) error {
	s.runningJobs = append(s.runningJobs, jobID)
	if job := s.jobs[jobID]; job != nil {
		job.Status = store.WorkerJobStatusRunning
	}
	return nil
}

func (s *workerStoreStub) MarkJobCompleted(_ context.Context, jobID uuid.UUID, result []byte) error {
	if s.completed == nil {
		s.completed = make(map[uuid.UUID][]byte)
	}
	s.completed[jobID] = append([]byte(nil), result...)
	if job := s.jobs[jobID]; job != nil {
		job.Status = store.WorkerJobStatusCompleted
		job.Result = append([]byte(nil), result...)
	}
	return nil
}

func (s *workerStoreStub) SetWorkerStatus(_ context.Context, workerID, status string) error {
	if s.workerStatuses == nil {
		s.workerStatuses = make(map[string]string)
	}
	s.workerStatuses[workerID] = status
	for _, worker := range s.registered {
		if worker.WorkerID == workerID {
			worker.Status = status
		}
	}
	return nil
}

func (s *workerStoreStub) UpdateJobStatus(_ context.Context, jobID uuid.UUID, status string, result []byte) error {
	if s.jobStatusUpdates == nil {
		s.jobStatusUpdates = make(map[uuid.UUID]jobStatusUpdate)
	}
	copy := append([]byte(nil), result...)
	s.jobStatusUpdates[jobID] = jobStatusUpdate{status: status, result: copy}
	if job := s.jobs[jobID]; job != nil {
		job.Status = status
		job.Result = copy
	}
	return nil
}

func (s *workerStoreStub) GetJob(ctx context.Context, jobID uuid.UUID) (*store.WorkerJobData, error) {
	job := s.jobs[jobID]
	if job == nil {
		return nil, nil
	}
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID != uuid.Nil && job.TenantID != tenantID {
		return nil, nil
	}
	copy := *job
	return &copy, nil
}

func (s *workerTeamStoreStub) CreateTeam(context.Context, *store.TeamData) error { return nil }
func (s *workerTeamStoreStub) GetTeam(context.Context, uuid.UUID) (*store.TeamData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) GetTeamUnscoped(context.Context, uuid.UUID) (*store.TeamData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) UpdateTeam(context.Context, uuid.UUID, map[string]any) error {
	return nil
}
func (s *workerTeamStoreStub) DeleteTeam(context.Context, uuid.UUID) error         { return nil }
func (s *workerTeamStoreStub) ListTeams(context.Context) ([]store.TeamData, error) { return nil, nil }
func (s *workerTeamStoreStub) AddMember(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (s *workerTeamStoreStub) RemoveMember(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s *workerTeamStoreStub) ListMembers(context.Context, uuid.UUID) ([]store.TeamMemberData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) ListIdleMembers(context.Context, uuid.UUID) ([]store.TeamMemberData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) GetTeamForAgent(context.Context, uuid.UUID) (*store.TeamData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) KnownUserIDs(context.Context, uuid.UUID, int) ([]string, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) ListTaskScopes(context.Context, uuid.UUID) ([]store.ScopeEntry, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) CreateTask(context.Context, *store.TeamTaskData) error { return nil }
func (s *workerTeamStoreStub) UpdateTask(context.Context, uuid.UUID, map[string]any) error {
	return nil
}
func (s *workerTeamStoreStub) ListTasks(context.Context, uuid.UUID, string, string, string, string, string, int, int) ([]store.TeamTaskData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) GetTask(_ context.Context, taskID uuid.UUID) (*store.TeamTaskData, error) {
	if s.task != nil && s.task.ID == taskID {
		cp := *s.task
		return &cp, nil
	}
	return nil, nil
}
func (s *workerTeamStoreStub) GetTasksByIDs(context.Context, []uuid.UUID) ([]store.TeamTaskData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) SearchTasks(context.Context, uuid.UUID, string, int, string) ([]store.TeamTaskData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) DeleteTask(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s *workerTeamStoreStub) DeleteTasks(context.Context, []uuid.UUID, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) ClaimTask(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *workerTeamStoreStub) AssignTask(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *workerTeamStoreStub) CompleteTask(_ context.Context, taskID, teamID uuid.UUID, result string) error {
	s.completeCalls = append(s.completeCalls, teamTaskTerminalCall{taskID: taskID, teamID: teamID, text: result})
	return nil
}
func (s *workerTeamStoreStub) CancelTask(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (s *workerTeamStoreStub) FailTask(_ context.Context, taskID, teamID uuid.UUID, errMsg string) error {
	s.failCalls = append(s.failCalls, teamTaskTerminalCall{taskID: taskID, teamID: teamID, text: errMsg})
	return nil
}
func (s *workerTeamStoreStub) FailPendingTask(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (s *workerTeamStoreStub) ReviewTask(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s *workerTeamStoreStub) ApproveTask(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (s *workerTeamStoreStub) RejectTask(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (s *workerTeamStoreStub) UpdateTaskProgress(_ context.Context, taskID, teamID uuid.UUID, percent int, step string) error {
	s.progressCalls = append(s.progressCalls, teamTaskProgressCall{taskID: taskID, teamID: teamID, percent: percent, step: step})
	return nil
}
func (s *workerTeamStoreStub) RenewTaskLock(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s *workerTeamStoreStub) ResetTaskStatus(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *workerTeamStoreStub) ListActiveTasksByChatID(context.Context, string) ([]store.TeamTaskData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) AddTaskComment(context.Context, *store.TeamTaskCommentData) error {
	return nil
}
func (s *workerTeamStoreStub) ListTaskComments(context.Context, uuid.UUID) ([]store.TeamTaskCommentData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) ListRecentTaskComments(context.Context, uuid.UUID, int) ([]store.TeamTaskCommentData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) RecordTaskEvent(context.Context, *store.TeamTaskEventData) error {
	return nil
}
func (s *workerTeamStoreStub) ListTaskEvents(context.Context, uuid.UUID) ([]store.TeamTaskEventData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) ListTeamEvents(context.Context, uuid.UUID, int, int) ([]store.TeamTaskEventData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) AttachFileToTask(context.Context, *store.TeamTaskAttachmentData) error {
	return nil
}
func (s *workerTeamStoreStub) GetAttachment(context.Context, uuid.UUID) (*store.TeamTaskAttachmentData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) ListTaskAttachments(context.Context, uuid.UUID) ([]store.TeamTaskAttachmentData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) DetachFileFromTask(context.Context, uuid.UUID, string) error {
	return nil
}
func (s *workerTeamStoreStub) RecoverAllStaleTasks(context.Context) ([]store.RecoveredTaskInfo, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) ForceRecoverAllTasks(context.Context) ([]store.RecoveredTaskInfo, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) ListRecoverableTasks(context.Context, uuid.UUID) ([]store.TeamTaskData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) MarkAllStaleTasks(context.Context, time.Time) ([]store.RecoveredTaskInfo, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) MarkInReviewStaleTasks(context.Context, time.Time) ([]store.RecoveredTaskInfo, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) FixOrphanedBlockedTasks(context.Context) ([]store.RecoveredTaskInfo, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) SetTaskFollowup(context.Context, uuid.UUID, uuid.UUID, time.Time, int, string, string, string) error {
	return nil
}
func (s *workerTeamStoreStub) ClearTaskFollowup(context.Context, uuid.UUID) error { return nil }
func (s *workerTeamStoreStub) ListAllFollowupDueTasks(context.Context) ([]store.TeamTaskData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) IncrementFollowupCount(context.Context, uuid.UUID, *time.Time) error {
	return nil
}
func (s *workerTeamStoreStub) ClearFollowupByScope(context.Context, string, string) (int, error) {
	return 0, nil
}
func (s *workerTeamStoreStub) SetFollowupForActiveTasks(context.Context, uuid.UUID, string, string, time.Time, int, string) (int, error) {
	return 0, nil
}
func (s *workerTeamStoreStub) HasActiveMemberTasks(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}
func (s *workerTeamStoreStub) GrantTeamAccess(context.Context, uuid.UUID, string, string, string) error {
	return nil
}
func (s *workerTeamStoreStub) RevokeTeamAccess(context.Context, uuid.UUID, string) error { return nil }
func (s *workerTeamStoreStub) ListTeamGrants(context.Context, uuid.UUID) ([]store.TeamUserGrant, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) ListUserTeams(context.Context, string) ([]store.TeamData, error) {
	return nil, nil
}
func (s *workerTeamStoreStub) HasTeamAccess(context.Context, uuid.UUID, string) (bool, error) {
	return false, nil
}

func TestWorkersRegister_StoresOnlineWorker(t *testing.T) {
	tenantID := uuid.New()
	workerStore := &workerStoreStub{}
	manager := localworker.NewManager()
	methods := NewWorkersMethods(workerStore, manager, nil, nil, nil)
	client := responseClient()
	setClientTenantID(t, client, tenantID)

	methods.handleRegister(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
		"displayName": "Baominh MacBook",
	}))

	resp := readResponse(t, client)
	if !resp.OK {
		t.Fatalf("expected ok response, got %#v", resp.Error)
	}
	if len(workerStore.registered) != 1 {
		t.Fatalf("registered workers = %d, want 1", len(workerStore.registered))
	}
	if got := workerStore.registered[0].TenantID; got != tenantID {
		t.Fatalf("tenant_id = %s, want %s", got, tenantID)
	}
	if got := workerStore.registered[0].Status; got != store.WorkerStatusOnline {
		t.Fatalf("status = %q, want %q", got, store.WorkerStatusOnline)
	}
	if reg, ok := manager.Get(tenantID, "worker-123"); !ok || reg == nil {
		t.Fatal("worker was not registered with localworker manager")
	}
	if got := getRegisteredWorkerID(t, client); got != "worker-123" {
		t.Fatalf("client registered worker = %q, want %q", got, "worker-123")
	}
}

func TestWorkersJobOutputRejectsWrongWorker(t *testing.T) {
	tenantID := uuid.New()
	jobID := uuid.New()
	workerStore := &workerStoreStub{
		jobs: map[uuid.UUID]*store.WorkerJobData{
			jobID: {
				BaseModel: store.BaseModel{ID: jobID},
				TenantID:  tenantID,
				WorkerID:  "worker-999",
				Status:    store.WorkerJobStatusQueued,
			},
		},
	}
	methods := NewWorkersMethods(workerStore, localworker.NewManager(), nil, nil, nil)
	client := responseClient()
	setClientTenantID(t, client, tenantID)

	methods.handleRegister(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, client)

	methods.handleJobOutput(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersJobOutput, map[string]any{
		"jobId":  jobID.String(),
		"output": map[string]any{"line": "hello"},
	}))

	resp := readResponse(t, client)
	if resp.OK {
		t.Fatal("expected error response for wrong worker")
	}
	if resp.Error == nil {
		t.Fatal("expected error details")
	}
	if resp.Error.Code != protocol.ErrUnauthorized {
		t.Fatalf("error code = %q, want %q", resp.Error.Code, protocol.ErrUnauthorized)
	}
	if !strings.Contains(resp.Error.Message, "does not own") {
		t.Fatalf("error message = %q, want ownership failure", resp.Error.Message)
	}
}

func TestWorkersJobStartedAcceptsBoundWorker(t *testing.T) {
	tenantID := uuid.New()
	jobID := uuid.New()
	workerStore := &workerStoreStub{
		jobs: map[uuid.UUID]*store.WorkerJobData{
			jobID: {
				BaseModel: store.BaseModel{ID: jobID},
				TenantID:  tenantID,
				WorkerID:  "worker-123",
				Status:    store.WorkerJobStatusQueued,
			},
		},
	}
	methods := NewWorkersMethods(workerStore, localworker.NewManager(), nil, nil, nil)
	client := responseClient()
	setClientTenantID(t, client, tenantID)

	methods.handleRegister(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, client)

	methods.handleJobStarted(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersJobStarted, map[string]any{
		"jobId": jobID.String(),
	}))

	resp := readResponse(t, client)
	if !resp.OK {
		t.Fatalf("expected ok response, got %#v", resp.Error)
	}
	if len(workerStore.runningJobs) != 1 || workerStore.runningJobs[0] != jobID {
		t.Fatalf("running jobs = %#v, want [%s]", workerStore.runningJobs, jobID)
	}
	if got := workerStore.jobs[jobID].Status; got != store.WorkerJobStatusRunning {
		t.Fatalf("job status = %q, want %q", got, store.WorkerJobStatusRunning)
	}
}

func TestWorkersJobStatusAcceptsBoundWorker(t *testing.T) {
	tenantID := uuid.New()
	jobID := uuid.New()
	workerStore := &workerStoreStub{
		jobs: map[uuid.UUID]*store.WorkerJobData{
			jobID: {
				BaseModel: store.BaseModel{ID: jobID},
				TenantID:  tenantID,
				WorkerID:  "worker-123",
				Status:    store.WorkerJobStatusRunning,
			},
		},
	}
	methods := NewWorkersMethods(workerStore, localworker.NewManager(), nil, nil, nil)
	client := responseClient()
	setClientTenantID(t, client, tenantID)

	methods.handleRegister(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, client)

	methods.handleJobStatus(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersJobStatus, map[string]any{
		"jobId":  jobID.String(),
		"status": "streaming",
		"detail": map[string]any{"phase": "tool_exec"},
	}))

	resp := readResponse(t, client)
	if !resp.OK {
		t.Fatalf("expected ok response, got %#v", resp.Error)
	}
	update, ok := workerStore.jobStatusUpdates[jobID]
	if !ok {
		t.Fatal("expected persisted job status update")
	}
	if update.status != "streaming" {
		t.Fatalf("status = %q, want %q", update.status, "streaming")
	}
	if !strings.Contains(string(update.result), "tool_exec") {
		t.Fatalf("result = %s, want payload to contain tool_exec", string(update.result))
	}
}

func TestWorkersJobFailedPersistsFailure(t *testing.T) {
	tenantID := uuid.New()
	jobID := uuid.New()
	workerStore := &workerStoreStub{
		jobs: map[uuid.UUID]*store.WorkerJobData{
			jobID: {
				BaseModel: store.BaseModel{ID: jobID},
				TenantID:  tenantID,
				WorkerID:  "worker-123",
				Status:    store.WorkerJobStatusRunning,
			},
		},
	}
	methods := NewWorkersMethods(workerStore, localworker.NewManager(), nil, nil, nil)
	client := responseClient()
	setClientTenantID(t, client, tenantID)

	methods.handleRegister(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, client)

	methods.handleJobFailed(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersJobFailed, map[string]any{
		"jobId": jobID.String(),
		"error": map[string]any{"message": "boom", "code": "EFAIL"},
	}))

	resp := readResponse(t, client)
	if !resp.OK {
		t.Fatalf("expected ok response, got %#v", resp.Error)
	}
	update, ok := workerStore.jobStatusUpdates[jobID]
	if !ok {
		t.Fatal("expected persisted failed job update")
	}
	if update.status != store.WorkerJobStatusFailed {
		t.Fatalf("status = %q, want %q", update.status, store.WorkerJobStatusFailed)
	}
	if !strings.Contains(string(update.result), "boom") {
		t.Fatalf("result = %s, want payload to contain boom", string(update.result))
	}
}

func TestWorkersJobOutputBroadcastsTaskProgress(t *testing.T) {
	tenantID := uuid.New()
	teamID := uuid.New()
	taskID := uuid.New()
	jobID := uuid.New()
	workerStore := &workerStoreStub{
		jobs: map[uuid.UUID]*store.WorkerJobData{
			jobID: {
				BaseModel: store.BaseModel{ID: jobID},
				TenantID:  tenantID,
				WorkerID:  "worker-123",
				TaskID:    &taskID,
				Status:    store.WorkerJobStatusRunning,
			},
		},
	}
	teamStore := &workerTeamStoreStub{task: &store.TeamTaskData{
		BaseModel:  store.BaseModel{ID: taskID},
		TeamID:     teamID,
		TaskNumber: 7,
		Subject:    "Sync worker stream",
		Status:     store.TeamTaskStatusInProgress,
		Channel:    "telegram",
		ChatID:     "chat-77",
		UserID:     "user-42",
		Metadata: map[string]any{
			tools.TaskMetaPeerKind: "group",
		},
	}}
	eventBus := &recordingEventPublisher{}
	methods := NewWorkersMethods(workerStore, localworker.NewManager(), teamStore, eventBus, nil)
	client := responseClient()
	setClientTenantID(t, client, tenantID)

	methods.handleRegister(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, client)

	methods.handleJobOutput(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersJobOutput, map[string]any{
		"jobId":  jobID.String(),
		"output": map[string]any{"delta": "Line 1", "tool": "bash"},
	}))

	resp := readResponse(t, client)
	if !resp.OK {
		t.Fatalf("expected ok response, got %#v", resp.Error)
	}
	if len(teamStore.progressCalls) != 0 {
		t.Fatalf("output should not persist team progress directly, got %#v", teamStore.progressCalls)
	}
	event := singleRecordedEvent(t, eventBus.events)
	if event.name != protocol.EventTeamTaskProgress {
		t.Fatalf("event name = %q, want %q", event.name, protocol.EventTeamTaskProgress)
	}
	if event.tenantID != tenantID {
		t.Fatalf("event tenant = %s, want %s", event.tenantID, tenantID)
	}
	payload, ok := event.payload.(protocol.TeamTaskEventPayload)
	if !ok {
		t.Fatalf("event payload type = %T, want protocol.TeamTaskEventPayload", event.payload)
	}
	if payload.TeamID != teamID.String() || payload.TaskID != taskID.String() {
		t.Fatalf("payload team/task = %s/%s, want %s/%s", payload.TeamID, payload.TaskID, teamID, taskID)
	}
	if payload.ProgressStep == "" || !strings.Contains(payload.ProgressStep, "Line 1") {
		t.Fatalf("progress step = %q, want output summary", payload.ProgressStep)
	}
	if payload.ProgressPercent != 0 {
		t.Fatalf("progress percent = %d, want 0", payload.ProgressPercent)
	}
	if payload.Channel != "telegram" || payload.ChatID != "chat-77" || payload.PeerKind != "group" || payload.UserID != "user-42" {
		t.Fatalf("payload context = %#v, want task context copied", payload)
	}
}

func TestWorkersJobStatusUpdatesLinkedTask(t *testing.T) {
	tenantID := uuid.New()
	teamID := uuid.New()
	taskID := uuid.New()
	jobID := uuid.New()
	workerStore := &workerStoreStub{
		jobs: map[uuid.UUID]*store.WorkerJobData{
			jobID: {
				BaseModel: store.BaseModel{ID: jobID},
				TenantID:  tenantID,
				WorkerID:  "worker-123",
				TaskID:    &taskID,
				Status:    store.WorkerJobStatusRunning,
			},
		},
	}
	teamStore := &workerTeamStoreStub{task: &store.TeamTaskData{
		BaseModel:       store.BaseModel{ID: taskID},
		TeamID:          teamID,
		TaskNumber:      8,
		Subject:         "Track streaming status",
		Status:          store.TeamTaskStatusInProgress,
		ProgressPercent: 15,
		OwnerAgentKey:   "member-1",
	}}
	eventBus := &recordingEventPublisher{}
	methods := NewWorkersMethods(workerStore, localworker.NewManager(), teamStore, eventBus, nil)
	client := responseClient()
	setClientTenantID(t, client, tenantID)

	methods.handleRegister(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, client)

	methods.handleJobStatus(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersJobStatus, map[string]any{
		"jobId":  jobID.String(),
		"status": "streaming",
		"detail": map[string]any{"phase": "tool_exec", "message": "Running tests", "progress": 44},
	}))

	resp := readResponse(t, client)
	if !resp.OK {
		t.Fatalf("expected ok response, got %#v", resp.Error)
	}
	update, ok := workerStore.jobStatusUpdates[jobID]
	if !ok {
		t.Fatal("expected persisted job status update")
	}
	if update.status != "streaming" {
		t.Fatalf("status = %q, want %q", update.status, "streaming")
	}
	if len(teamStore.progressCalls) != 1 {
		t.Fatalf("progress calls = %d, want 1", len(teamStore.progressCalls))
	}
	if teamStore.progressCalls[0].taskID != taskID || teamStore.progressCalls[0].teamID != teamID {
		t.Fatalf("progress call target = %#v, want task/team ids", teamStore.progressCalls[0])
	}
	if teamStore.progressCalls[0].percent != 44 {
		t.Fatalf("progress percent = %d, want 44", teamStore.progressCalls[0].percent)
	}
	if !strings.Contains(teamStore.progressCalls[0].step, "Running tests") {
		t.Fatalf("progress step = %q, want detail summary", teamStore.progressCalls[0].step)
	}
	event := singleRecordedEvent(t, eventBus.events)
	if event.name != protocol.EventTeamTaskProgress {
		t.Fatalf("event name = %q, want %q", event.name, protocol.EventTeamTaskProgress)
	}
}

func TestWorkersJobCompletedCompletesLinkedTask(t *testing.T) {
	tenantID := uuid.New()
	teamID := uuid.New()
	taskID := uuid.New()
	jobID := uuid.New()
	workerStore := &workerStoreStub{
		jobs: map[uuid.UUID]*store.WorkerJobData{
			jobID: {
				BaseModel: store.BaseModel{ID: jobID},
				TenantID:  tenantID,
				WorkerID:  "worker-123",
				TaskID:    &taskID,
				Status:    store.WorkerJobStatusRunning,
			},
		},
	}
	teamStore := &workerTeamStoreStub{task: &store.TeamTaskData{
		BaseModel:     store.BaseModel{ID: taskID},
		TeamID:        teamID,
		TaskNumber:    9,
		Subject:       "Finalize worker result",
		Status:        store.TeamTaskStatusInProgress,
		OwnerAgentKey: "member-1",
	}}
	eventBus := &recordingEventPublisher{}
	postTurn := &workerPostTurnStub{}
	methods := NewWorkersMethods(workerStore, localworker.NewManager(), teamStore, eventBus, postTurn)
	client := responseClient()
	setClientTenantID(t, client, tenantID)

	methods.handleRegister(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, client)

	methods.handleJobCompleted(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersJobCompleted, map[string]any{
		"jobId":  jobID.String(),
		"result": map[string]any{"summary": "All tests passed", "artifacts": []string{"report.txt"}},
	}))

	resp := readResponse(t, client)
	if !resp.OK {
		t.Fatalf("expected ok response, got %#v", resp.Error)
	}
	if len(teamStore.completeCalls) != 1 {
		t.Fatalf("complete calls = %d, want 1", len(teamStore.completeCalls))
	}
	if teamStore.completeCalls[0].taskID != taskID || teamStore.completeCalls[0].teamID != teamID {
		t.Fatalf("complete call target = %#v, want task/team ids", teamStore.completeCalls[0])
	}
	if !strings.Contains(teamStore.completeCalls[0].text, "All tests passed") {
		t.Fatalf("completion result = %q, want marshaled worker result", teamStore.completeCalls[0].text)
	}
	event := singleRecordedEvent(t, eventBus.events)
	if event.name != protocol.EventTeamTaskCompleted {
		t.Fatalf("event name = %q, want %q", event.name, protocol.EventTeamTaskCompleted)
	}
	if len(postTurn.dispatchedTeamIDs) != 1 || postTurn.dispatchedTeamIDs[0] != teamID {
		t.Fatalf("dispatched team IDs = %#v, want [%s]", postTurn.dispatchedTeamIDs, teamID)
	}
}

func TestWorkersJobFailedFailsLinkedTask(t *testing.T) {
	tenantID := uuid.New()
	teamID := uuid.New()
	taskID := uuid.New()
	jobID := uuid.New()
	workerStore := &workerStoreStub{
		jobs: map[uuid.UUID]*store.WorkerJobData{
			jobID: {
				BaseModel: store.BaseModel{ID: jobID},
				TenantID:  tenantID,
				WorkerID:  "worker-123",
				TaskID:    &taskID,
				Status:    store.WorkerJobStatusRunning,
			},
		},
	}
	teamStore := &workerTeamStoreStub{task: &store.TeamTaskData{
		BaseModel:  store.BaseModel{ID: taskID},
		TeamID:     teamID,
		TaskNumber: 10,
		Subject:    "Fail worker result",
		Status:     store.TeamTaskStatusInProgress,
	}}
	eventBus := &recordingEventPublisher{}
	postTurn := &workerPostTurnStub{}
	methods := NewWorkersMethods(workerStore, localworker.NewManager(), teamStore, eventBus, postTurn)
	client := responseClient()
	setClientTenantID(t, client, tenantID)

	methods.handleRegister(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, client)

	methods.handleJobFailed(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersJobFailed, map[string]any{
		"jobId": jobID.String(),
		"error": map[string]any{"message": "boom", "code": "EFAIL"},
	}))

	resp := readResponse(t, client)
	if !resp.OK {
		t.Fatalf("expected ok response, got %#v", resp.Error)
	}
	if len(teamStore.failCalls) != 1 {
		t.Fatalf("fail calls = %d, want 1", len(teamStore.failCalls))
	}
	if teamStore.failCalls[0].taskID != taskID || teamStore.failCalls[0].teamID != teamID {
		t.Fatalf("fail call target = %#v, want task/team ids", teamStore.failCalls[0])
	}
	if !strings.Contains(teamStore.failCalls[0].text, "boom") {
		t.Fatalf("failure reason = %q, want marshaled worker error", teamStore.failCalls[0].text)
	}
	event := singleRecordedEvent(t, eventBus.events)
	if event.name != protocol.EventTeamTaskFailed {
		t.Fatalf("event name = %q, want %q", event.name, protocol.EventTeamTaskFailed)
	}
	if len(postTurn.dispatchedTeamIDs) != 1 || postTurn.dispatchedTeamIDs[0] != teamID {
		t.Fatalf("dispatched team IDs = %#v, want [%s]", postTurn.dispatchedTeamIDs, teamID)
	}
}

func TestWorkersDisconnectMarksWorkerOffline(t *testing.T) {
	tenantID := uuid.New()
	workerStore := &workerStoreStub{}
	manager := localworker.NewManager()
	methods := NewWorkersMethods(workerStore, manager, nil, nil, nil)
	server := &gateway.Server{}
	setServerEventPublisher(t, server, bus.New())
	setServerClients(t, server, map[string]*gateway.Client{})
	setServerWorkerManager(t, server, manager)
	setServerWorkerStore(t, server, workerStore)
	client := responseClient()
	setClientTenantID(t, client, tenantID)
	setClientServer(t, client, server)

	methods.handleRegister(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, client)

	server.TestOnlyUnregisterClient(client)

	if got := workerStore.workerStatuses["worker-123"]; got != store.WorkerStatusOffline {
		t.Fatalf("worker status = %q, want %q", got, store.WorkerStatusOffline)
	}
	if _, ok := manager.Get(tenantID, "worker-123"); ok {
		t.Fatal("worker should be disconnected from manager")
	}
}

func TestWorkersStaleSocketCannotAckAfterTakeover(t *testing.T) {
	tenantID := uuid.New()
	jobID := uuid.New()
	workerStore := &workerStoreStub{
		jobs: map[uuid.UUID]*store.WorkerJobData{
			jobID: {
				BaseModel: store.BaseModel{ID: jobID},
				TenantID:  tenantID,
				WorkerID:  "worker-123",
				Status:    store.WorkerJobStatusRunning,
			},
		},
	}
	manager := localworker.NewManager()
	methods := NewWorkersMethods(workerStore, manager, nil, nil, nil)
	oldClient := responseClient()
	newClient := responseClient()
	setClientTenantID(t, oldClient, tenantID)
	setClientTenantID(t, newClient, tenantID)

	methods.handleRegister(context.Background(), oldClient, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, oldClient)

	methods.handleRegister(context.Background(), newClient, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, newClient)

	methods.handleJobStatus(context.Background(), oldClient, buildWorkerRequest(t, protocol.MethodWorkersJobStatus, map[string]any{
		"jobId":  jobID.String(),
		"status": "streaming",
		"detail": map[string]any{"phase": "tool_exec"},
	}))

	resp := readResponse(t, oldClient)
	if resp.OK {
		t.Fatal("expected stale socket callback to be rejected")
	}
	if resp.Error == nil {
		t.Fatal("expected error details")
	}
	if resp.Error.Code != protocol.ErrUnauthorized {
		t.Fatalf("error code = %q, want %q", resp.Error.Code, protocol.ErrUnauthorized)
	}
	if _, ok := workerStore.jobStatusUpdates[jobID]; ok {
		t.Fatal("stale socket should not persist callback updates")
	}
}

func TestWorkersStaleDisconnectDoesNotRemoveNewerBinding(t *testing.T) {
	tenantID := uuid.New()
	workerStore := &workerStoreStub{}
	manager := localworker.NewManager()
	methods := NewWorkersMethods(workerStore, manager, nil, nil, nil)
	server := &gateway.Server{}
	setServerEventPublisher(t, server, bus.New())
	setServerClients(t, server, map[string]*gateway.Client{})
	setServerWorkerManager(t, server, manager)
	setServerWorkerStore(t, server, workerStore)

	clientA := responseClient()
	setClientTenantID(t, clientA, tenantID)
	setClientServer(t, clientA, server)
	methods.handleRegister(context.Background(), clientA, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, clientA)

	clientB := responseClient()
	setClientTenantID(t, clientB, tenantID)
	setClientServer(t, clientB, server)
	methods.handleRegister(context.Background(), clientB, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, clientB)

	server.TestOnlyUnregisterClient(clientA)

	if got := workerStore.workerStatuses["worker-123"]; got != store.WorkerStatusOnline {
		t.Fatalf("worker status after stale disconnect = %q, want %q", got, store.WorkerStatusOnline)
	}
	reg, ok := manager.Get(tenantID, "worker-123")
	if !ok || reg == nil {
		t.Fatal("newer binding should remain registered")
	}
	if reg.Connection != clientB {
		t.Fatal("newer client binding was replaced by stale disconnect")
	}

	server.TestOnlyUnregisterClient(clientB)
	if got := workerStore.workerStatuses["worker-123"]; got != store.WorkerStatusOffline {
		t.Fatalf("worker status after active disconnect = %q, want %q", got, store.WorkerStatusOffline)
	}
}

func TestWorkersReRegisterDifferentWorkerCleansUpOldBinding(t *testing.T) {
	tenantID := uuid.New()
	workerStore := &workerStoreStub{}
	manager := localworker.NewManager()
	methods := NewWorkersMethods(workerStore, manager, nil, nil, nil)
	client := responseClient()
	setClientTenantID(t, client, tenantID)

	methods.handleRegister(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-123",
		"runtimeKind": "wails_desktop",
	}))
	_ = readResponse(t, client)

	methods.handleRegister(context.Background(), client, buildWorkerRequest(t, protocol.MethodWorkersRegister, map[string]any{
		"workerId":    "worker-456",
		"runtimeKind": "wails_desktop",
	}))
	resp := readResponse(t, client)
	if !resp.OK {
		t.Fatalf("expected ok response, got %#v", resp.Error)
	}

	if got := workerStore.workerStatuses["worker-123"]; got != store.WorkerStatusOffline {
		t.Fatalf("old worker status = %q, want %q", got, store.WorkerStatusOffline)
	}
	if _, ok := manager.Get(tenantID, "worker-123"); ok {
		t.Fatal("old worker binding should be removed after re-register")
	}
	if got := workerStore.workerStatuses["worker-456"]; got != store.WorkerStatusOnline {
		t.Fatalf("new worker status = %q, want %q", got, store.WorkerStatusOnline)
	}
	reg, ok := manager.Get(tenantID, "worker-456")
	if !ok || reg == nil {
		t.Fatal("new worker binding should be registered")
	}
	if reg.Connection != client {
		t.Fatal("new worker binding should point to the registering client")
	}
	if got := getRegisteredWorkerID(t, client); got != "worker-456" {
		t.Fatalf("client registered worker = %q, want %q", got, "worker-456")
	}
}

func buildWorkerRequest(t *testing.T, method string, params map[string]any) *protocol.RequestFrame {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return &protocol.RequestFrame{
		ID:     "test-worker-req-1",
		Method: method,
		Params: raw,
	}
}

func setClientTenantID(t *testing.T, client *gateway.Client, tenantID uuid.UUID) {
	t.Helper()
	field := reflect.ValueOf(client).Elem().FieldByName("tenantID")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(tenantID))
}

func getRegisteredWorkerID(t *testing.T, client *gateway.Client) string {
	t.Helper()
	field := reflect.ValueOf(client).Elem().FieldByName("registeredWorkerID")
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().String()
}

func setClientServer(t *testing.T, client *gateway.Client, server *gateway.Server) {
	t.Helper()
	field := reflect.ValueOf(client).Elem().FieldByName("server")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(server))
}

func setServerWorkerManager(t *testing.T, server *gateway.Server, manager *localworker.Manager) {
	t.Helper()
	field := reflect.ValueOf(server).Elem().FieldByName("workers")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(manager))
}

func setServerWorkerStore(t *testing.T, server *gateway.Server, workerStore store.WorkerStore) {
	t.Helper()
	field := reflect.ValueOf(server).Elem().FieldByName("workerStore")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(workerStore))
}

func setServerEventPublisher(t *testing.T, server *gateway.Server, eventPub bus.EventPublisher) {
	t.Helper()
	field := reflect.ValueOf(server).Elem().FieldByName("eventPub")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(eventPub))
}

func setServerClients(t *testing.T, server *gateway.Server, clients map[string]*gateway.Client) {
	t.Helper()
	field := reflect.ValueOf(server).Elem().FieldByName("clients")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(clients))
}

func singleRecordedEvent(t *testing.T, events []recordedBusEvent) recordedBusEvent {
	t.Helper()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	return events[0]
}
