package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/localworker"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestRunRequest_LocalWorkerMember_DispatchesJobInsteadOfProvider(t *testing.T) {
	tenantID := uuid.New()
	agentID := uuid.New()
	provider := &localWorkerTestProvider{}
	workerStore := &localWorkerTestStore{}
	manager := localworker.NewManager()
	conn := &localWorkerTestConnection{}
	if _, err := manager.Register(tenantID, "worker-123", conn); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	loop := newLocalWorkerTestLoop(t, provider, tenantID, agentID)
	setLoopStringField(t, loop, "executionMode", store.AgentExecutionModeLocalWorker)
	setLoopStringField(t, loop, "localRuntimeKind", "wails_desktop")
	setLoopStringField(t, loop, "boundWorkerID", "worker-123")
	setLoopDispatcherField(t, loop, manager, workerStore)

	req := RunRequest{
		SessionKey:        "agent:test:ws:direct:chat-1",
		Message:           "Implement the fix",
		Channel:           "ws",
		ChannelType:       "web",
		ChatID:            "chat-1",
		PeerKind:          "direct",
		RunID:             "run-local-worker",
		UserID:            "user-123",
		SenderID:          "sender-123",
		TeamID:            uuid.NewString(),
		TeamTaskID:        uuid.NewString(),
		ParentAgentID:     "leader-agent",
		LeaderAgentID:     uuid.NewString(),
		WorkspaceChannel:  "telegram",
		WorkspaceChatID:   "room-7",
		TeamWorkspace:     t.TempDir(),
		ExtraSystemPrompt: "Stay concise",
		LocalKey:          "room-7:topic:2",
	}

	result, err := loop.Run(store.WithTenantID(context.Background(), tenantID), req)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Run returned nil result")
	}
	if result.RunID != req.RunID {
		t.Fatalf("RunID = %q, want %q", result.RunID, req.RunID)
	}
	if result.Iterations != 0 {
		t.Fatalf("Iterations = %d, want 0 for queued worker run", result.Iterations)
	}
	if provider.chatCalls != 0 || provider.chatStreamCalls != 0 {
		t.Fatalf("provider should not be used, got chat=%d stream=%d", provider.chatCalls, provider.chatStreamCalls)
	}
	if len(workerStore.jobs) != 1 {
		t.Fatalf("created jobs = %d, want 1", len(workerStore.jobs))
	}
	job := workerStore.jobs[0]
	if job.TenantID != tenantID {
		t.Fatalf("job tenant_id = %s, want %s", job.TenantID, tenantID)
	}
	if job.WorkerID != "worker-123" {
		t.Fatalf("job worker_id = %q, want %q", job.WorkerID, "worker-123")
	}
	if job.Status != store.WorkerJobStatusQueued {
		t.Fatalf("job status = %q, want %q", job.Status, store.WorkerJobStatusQueued)
	}
	if job.JobType != "run_task" {
		t.Fatalf("job type = %q, want %q", job.JobType, "run_task")
	}
	if job.AgentID == nil || *job.AgentID != agentID {
		t.Fatalf("job agent_id = %v, want %s", job.AgentID, agentID)
	}
	taskID, err := uuid.Parse(req.TeamTaskID)
	if err != nil {
		t.Fatalf("parse task id: %v", err)
	}
	if job.TaskID == nil || *job.TaskID != taskID {
		t.Fatalf("job task_id = %v, want %s", job.TaskID, taskID)
	}
	if len(conn.envelopes) != 1 {
		t.Fatalf("dispatch envelopes = %d, want 1", len(conn.envelopes))
	}
	if conn.envelopes[0].Type != "job.dispatch" {
		t.Fatalf("dispatch type = %q, want %q", conn.envelopes[0].Type, "job.dispatch")
	}
	if conn.envelopes[0].TenantID != tenantID {
		t.Fatalf("dispatch tenant_id = %s, want %s", conn.envelopes[0].TenantID, tenantID)
	}
	if conn.envelopes[0].WorkerID != "worker-123" {
		t.Fatalf("dispatch worker_id = %q, want %q", conn.envelopes[0].WorkerID, "worker-123")
	}

	envPayload, ok := conn.envelopes[0].Payload.(map[string]any)
	if !ok {
		t.Fatalf("dispatch payload type = %T, want map[string]any", conn.envelopes[0].Payload)
	}
	if got := envPayload["jobId"]; got != job.ID.String() {
		t.Fatalf("payload jobId = %#v, want %q", got, job.ID.String())
	}
	if got := envPayload["runtimeKind"]; got != "wails_desktop" {
		t.Fatalf("payload runtimeKind = %#v, want %q", got, "wails_desktop")
	}

	var persistedPayload localWorkerDispatchPayload
	if err := json.Unmarshal(job.Payload, &persistedPayload); err != nil {
		t.Fatalf("unmarshal job payload: %v", err)
	}
	if persistedPayload.RunID != req.RunID {
		t.Fatalf("payload run_id = %q, want %q", persistedPayload.RunID, req.RunID)
	}
	if persistedPayload.SessionKey != req.SessionKey {
		t.Fatalf("payload session_key = %q, want %q", persistedPayload.SessionKey, req.SessionKey)
	}
	if persistedPayload.AgentID != agentID.String() {
		t.Fatalf("payload agent_id = %q, want %q", persistedPayload.AgentID, agentID.String())
	}
	if persistedPayload.AgentKey != loop.ID() {
		t.Fatalf("payload agent_key = %q, want %q", persistedPayload.AgentKey, loop.ID())
	}
	if persistedPayload.UserID != req.UserID {
		t.Fatalf("payload user_id = %q, want %q", persistedPayload.UserID, req.UserID)
	}
	if persistedPayload.Message != req.Message {
		t.Fatalf("payload message = %q, want %q", persistedPayload.Message, req.Message)
	}
	if persistedPayload.Channel != req.Channel || persistedPayload.ChannelType != req.ChannelType {
		t.Fatalf("payload channel info = %#v", persistedPayload)
	}
	if persistedPayload.ChatID != req.ChatID || persistedPayload.PeerKind != req.PeerKind {
		t.Fatalf("payload chat routing = %#v", persistedPayload)
	}
	if persistedPayload.TeamID != req.TeamID || persistedPayload.TeamTaskID != req.TeamTaskID {
		t.Fatalf("payload team routing = %#v", persistedPayload)
	}
	if persistedPayload.ParentAgentID != req.ParentAgentID || persistedPayload.LeaderAgentID != req.LeaderAgentID {
		t.Fatalf("payload delegation routing = %#v", persistedPayload)
	}
	if persistedPayload.LocalKey != req.LocalKey {
		t.Fatalf("payload local_key = %q, want %q", persistedPayload.LocalKey, req.LocalKey)
	}
	if persistedPayload.WorkspaceChannel != req.WorkspaceChannel || persistedPayload.WorkspaceChatID != req.WorkspaceChatID {
		t.Fatalf("payload workspace routing = %#v", persistedPayload)
	}
	if persistedPayload.TeamWorkspace != req.TeamWorkspace {
		t.Fatalf("payload team_workspace = %q, want %q", persistedPayload.TeamWorkspace, req.TeamWorkspace)
	}
	if persistedPayload.ExtraSystemPrompt != req.ExtraSystemPrompt {
		t.Fatalf("payload extra_system_prompt = %q, want %q", persistedPayload.ExtraSystemPrompt, req.ExtraSystemPrompt)
	}
	if persistedPayload.RunContext == nil {
		t.Fatal("payload run_context is nil")
	}
	if persistedPayload.RunContext.Workspace != req.TeamWorkspace {
		t.Fatalf("run_context.workspace = %q, want %q", persistedPayload.RunContext.Workspace, req.TeamWorkspace)
	}
	if persistedPayload.RunContext.TeamWorkspace != req.TeamWorkspace {
		t.Fatalf("run_context.team_workspace = %q, want %q", persistedPayload.RunContext.TeamWorkspace, req.TeamWorkspace)
	}
	if persistedPayload.RunContext.TeamID != req.TeamID {
		t.Fatalf("run_context.team_id = %q, want %q", persistedPayload.RunContext.TeamID, req.TeamID)
	}
	if persistedPayload.RunContext.TeamTaskID != req.TeamTaskID {
		t.Fatalf("run_context.team_task_id = %q, want %q", persistedPayload.RunContext.TeamTaskID, req.TeamTaskID)
	}
	if persistedPayload.RunContext.WorkspaceChannel != req.WorkspaceChannel || persistedPayload.RunContext.WorkspaceChatID != req.WorkspaceChatID {
		t.Fatalf("run_context workspace routing = %#v", persistedPayload.RunContext)
	}
	if persistedPayload.RunContext.AgentID != agentID {
		t.Fatalf("run_context.agent_id = %s, want %s", persistedPayload.RunContext.AgentID, agentID)
	}
	if persistedPayload.RunContext.TenantID != tenantID {
		t.Fatalf("run_context.tenant_id = %s, want %s", persistedPayload.RunContext.TenantID, tenantID)
	}
	if persistedPayload.RunContext.UserID != req.UserID {
		t.Fatalf("run_context.user_id = %q, want %q", persistedPayload.RunContext.UserID, req.UserID)
	}
	if persistedPayload.RunContext.AgentKey != loop.ID() {
		t.Fatalf("run_context.agent_key = %q, want %q", persistedPayload.RunContext.AgentKey, loop.ID())
	}
	if persistedPayload.RunContext.AgentToolKey != loop.ID() {
		t.Fatalf("run_context.agent_tool_key = %q, want %q", persistedPayload.RunContext.AgentToolKey, loop.ID())
	}
	if persistedPayload.RunContext.ChannelType != req.ChannelType {
		t.Fatalf("run_context.channel_type = %q, want %q", persistedPayload.RunContext.ChannelType, req.ChannelType)
	}
}

func TestRunRequest_LocalWorkerMember_FailsWhenWorkerOffline(t *testing.T) {
	tenantID := uuid.New()
	provider := &localWorkerTestProvider{}
	workerStore := &localWorkerTestStore{}

	loop := newLocalWorkerTestLoop(t, provider, tenantID, uuid.New())
	setLoopStringField(t, loop, "executionMode", store.AgentExecutionModeLocalWorker)
	setLoopStringField(t, loop, "localRuntimeKind", "wails_desktop")
	setLoopStringField(t, loop, "boundWorkerID", "worker-offline")
	setLoopDispatcherField(t, loop, localworker.NewManager(), workerStore)

	_, err := loop.Run(store.WithTenantID(context.Background(), tenantID), RunRequest{
		SessionKey:  "agent:test:ws:direct:chat-2",
		Message:     "Run this on my laptop",
		Channel:     "ws",
		ChannelType: "web",
		ChatID:      "chat-2",
		PeerKind:    "direct",
		RunID:       "run-offline-worker",
		UserID:      "user-999",
	})
	if err == nil {
		t.Fatal("expected offline worker error")
	}
	if !errors.Is(err, localworker.ErrWorkerNotConnected) && !strings.Contains(err.Error(), localworker.ErrWorkerNotConnected.Error()) {
		t.Fatalf("error = %v, want local worker offline error", err)
	}
	if provider.chatCalls != 0 || provider.chatStreamCalls != 0 {
		t.Fatalf("provider should not be used, got chat=%d stream=%d", provider.chatCalls, provider.chatStreamCalls)
	}
	if len(workerStore.jobs) != 0 {
		t.Fatalf("created jobs = %d, want 0 when worker offline", len(workerStore.jobs))
	}
}

func TestRunRequest_LocalWorkerMember_MarksJobFailedWhenDispatchErrors(t *testing.T) {
	tenantID := uuid.New()
	agentID := uuid.New()
	provider := &localWorkerTestProvider{}
	workerStore := &localWorkerTestStore{}
	manager := localworker.NewManager()
	conn := &localWorkerFailingConnection{err: errors.New("socket write failed")}
	if _, err := manager.Register(tenantID, "worker-123", conn); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	loop := newLocalWorkerTestLoop(t, provider, tenantID, agentID)
	setLoopStringField(t, loop, "executionMode", store.AgentExecutionModeLocalWorker)
	setLoopStringField(t, loop, "localRuntimeKind", "wails_desktop")
	setLoopStringField(t, loop, "boundWorkerID", "worker-123")
	setLoopDispatcherField(t, loop, manager, workerStore)

	_, err := loop.Run(store.WithTenantID(context.Background(), tenantID), RunRequest{
		SessionKey:  "agent:test:ws:direct:chat-3",
		Message:     "Dispatch should fail",
		Channel:     "ws",
		ChannelType: "web",
		ChatID:      "chat-3",
		PeerKind:    "direct",
		RunID:       "run-dispatch-error",
		UserID:      "user-123",
	})
	if err == nil {
		t.Fatal("expected dispatch error")
	}
	if len(workerStore.jobs) != 1 {
		t.Fatalf("created jobs = %d, want 1", len(workerStore.jobs))
	}
	job := workerStore.jobs[0]
	if job.Status != store.WorkerJobStatusFailed {
		t.Fatalf("job status = %q, want %q", job.Status, store.WorkerJobStatusFailed)
	}
	if !strings.Contains(string(job.Result), "socket write failed") {
		t.Fatalf("job result = %q, want dispatch failure details", string(job.Result))
	}
	if provider.chatCalls != 0 || provider.chatStreamCalls != 0 {
		t.Fatalf("provider should not be used, got chat=%d stream=%d", provider.chatCalls, provider.chatStreamCalls)
	}
}

type localWorkerTestProvider struct {
	chatCalls       int
	chatStreamCalls int
}

func (p *localWorkerTestProvider) Chat(context.Context, providers.ChatRequest) (*providers.ChatResponse, error) {
	p.chatCalls++
	return &providers.ChatResponse{Content: "provider should not be called", FinishReason: "stop"}, nil
}

func (p *localWorkerTestProvider) ChatStream(context.Context, providers.ChatRequest, func(providers.StreamChunk)) (*providers.ChatResponse, error) {
	p.chatStreamCalls++
	return &providers.ChatResponse{Content: "provider should not be called", FinishReason: "stop"}, nil
}

func (p *localWorkerTestProvider) DefaultModel() string { return "test-model" }
func (p *localWorkerTestProvider) Name() string         { return "test-provider" }

type localWorkerTestStore struct {
	jobs []*store.WorkerJobData
}

func (s *localWorkerTestStore) Register(context.Context, *store.WorkerData) error     { return nil }
func (s *localWorkerTestStore) SetWorkerStatus(context.Context, string, string) error { return nil }
func (s *localWorkerTestStore) GetWorker(context.Context, string) (*store.WorkerData, error) {
	return nil, nil
}
func (s *localWorkerTestStore) MarkJobRunning(context.Context, uuid.UUID) error           { return nil }
func (s *localWorkerTestStore) MarkJobCompleted(context.Context, uuid.UUID, []byte) error { return nil }
func (s *localWorkerTestStore) UpdateJobStatus(_ context.Context, jobID uuid.UUID, status string, result []byte) error {
	for _, job := range s.jobs {
		if job.ID != jobID {
			continue
		}
		job.Status = status
		job.Result = append([]byte(nil), result...)
		return nil
	}
	return nil
}
func (s *localWorkerTestStore) GetJob(context.Context, uuid.UUID) (*store.WorkerJobData, error) {
	return nil, nil
}

func (s *localWorkerTestStore) CreateJob(_ context.Context, job *store.WorkerJobData) error {
	copy := *job
	copy.Payload = append([]byte(nil), job.Payload...)
	copy.Result = append([]byte(nil), job.Result...)
	s.jobs = append(s.jobs, &copy)
	return nil
}

type localWorkerTestConnection struct {
	envelopes []localworker.Envelope
}

func (c *localWorkerTestConnection) Dispatch(_ context.Context, env localworker.Envelope) error {
	c.envelopes = append(c.envelopes, env)
	return nil
}

type localWorkerFailingConnection struct{ err error }

func (c *localWorkerFailingConnection) Dispatch(context.Context, localworker.Envelope) error {
	return c.err
}

type localWorkerDispatchPayload struct {
	RunID             string            `json:"runId"`
	SessionKey        string            `json:"sessionKey"`
	AgentID           string            `json:"agentId"`
	AgentKey          string            `json:"agentKey"`
	UserID            string            `json:"userId,omitempty"`
	Message           string            `json:"message"`
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
}

func newLocalWorkerTestLoop(t *testing.T, provider providers.Provider, tenantID, agentID uuid.UUID) *Loop {
	t.Helper()
	sessStore := newLocalWorkerSessionStore()
	loop := NewLoop(LoopConfig{
		ID:            "member-local-worker",
		AgentUUID:     agentID,
		TenantID:      tenantID,
		AgentType:     store.AgentTypeOpen,
		Provider:      provider,
		Model:         "test-model",
		Sessions:      sessStore,
		MaxIterations: 1,
		OnEvent:       func(AgentEvent) {},
	})
	return loop
}

type localWorkerSessionStore struct {
	sessions map[string]*store.SessionData
}

func newLocalWorkerSessionStore() *localWorkerSessionStore {
	return &localWorkerSessionStore{sessions: make(map[string]*store.SessionData)}
}

func (s *localWorkerSessionStore) GetOrCreate(_ context.Context, key string) *store.SessionData {
	if sess := s.sessions[key]; sess != nil {
		return sess
	}
	now := time.Now()
	sess := &store.SessionData{Key: key, Created: now, Updated: now}
	s.sessions[key] = sess
	return sess
}

func (s *localWorkerSessionStore) Get(_ context.Context, key string) *store.SessionData {
	return s.sessions[key]
}

func (s *localWorkerSessionStore) AddMessage(_ context.Context, key string, msg providers.Message) {
	sess := s.GetOrCreate(context.Background(), key)
	sess.Messages = append(sess.Messages, msg)
}

func (s *localWorkerSessionStore) GetHistory(_ context.Context, key string) []providers.Message {
	if sess := s.sessions[key]; sess != nil {
		cp := make([]providers.Message, len(sess.Messages))
		copy(cp, sess.Messages)
		return cp
	}
	return nil
}

func (s *localWorkerSessionStore) GetSummary(_ context.Context, key string) string {
	if sess := s.sessions[key]; sess != nil {
		return sess.Summary
	}
	return ""
}

func (s *localWorkerSessionStore) SetSummary(_ context.Context, key, summary string) {
	sess := s.GetOrCreate(context.Background(), key)
	sess.Summary = summary
}

func (s *localWorkerSessionStore) GetLabel(_ context.Context, key string) string {
	if sess := s.sessions[key]; sess != nil {
		return sess.Label
	}
	return ""
}

func (s *localWorkerSessionStore) SetLabel(_ context.Context, key, label string) {
	sess := s.GetOrCreate(context.Background(), key)
	sess.Label = label
}

func (s *localWorkerSessionStore) SetAgentInfo(_ context.Context, key string, agentUUID uuid.UUID, userID string) {
	sess := s.GetOrCreate(context.Background(), key)
	sess.AgentUUID = agentUUID
	sess.UserID = userID
}

func (s *localWorkerSessionStore) TruncateHistory(context.Context, string, int)            {}
func (s *localWorkerSessionStore) SetHistory(context.Context, string, []providers.Message) {}
func (s *localWorkerSessionStore) Reset(context.Context, string)                           {}
func (s *localWorkerSessionStore) Delete(context.Context, string) error                    { return nil }
func (s *localWorkerSessionStore) Save(context.Context, string) error                      { return nil }
func (s *localWorkerSessionStore) UpdateMetadata(context.Context, string, string, string, string) {
}
func (s *localWorkerSessionStore) AccumulateTokens(context.Context, string, int64, int64) {}
func (s *localWorkerSessionStore) IncrementCompaction(context.Context, string)            {}
func (s *localWorkerSessionStore) GetCompactionCount(context.Context, string) int         { return 0 }
func (s *localWorkerSessionStore) GetMemoryFlushCompactionCount(context.Context, string) int {
	return 0
}
func (s *localWorkerSessionStore) SetMemoryFlushDone(context.Context, string) {}
func (s *localWorkerSessionStore) GetSessionMetadata(context.Context, string) map[string]string {
	return nil
}
func (s *localWorkerSessionStore) SetSessionMetadata(context.Context, string, map[string]string) {
}
func (s *localWorkerSessionStore) SetSpawnInfo(context.Context, string, string, int) {}
func (s *localWorkerSessionStore) SetContextWindow(_ context.Context, key string, cw int) {
	sess := s.GetOrCreate(context.Background(), key)
	sess.ContextWindow = cw
}
func (s *localWorkerSessionStore) GetContextWindow(_ context.Context, key string) int {
	if sess := s.sessions[key]; sess != nil {
		return sess.ContextWindow
	}
	return 0
}
func (s *localWorkerSessionStore) SetLastPromptTokens(context.Context, string, int, int) {}
func (s *localWorkerSessionStore) GetLastPromptTokens(context.Context, string) (int, int) {
	return 0, 0
}
func (s *localWorkerSessionStore) List(context.Context, string) []store.SessionInfo { return nil }
func (s *localWorkerSessionStore) ListPaged(context.Context, store.SessionListOpts) store.SessionListResult {
	return store.SessionListResult{}
}
func (s *localWorkerSessionStore) ListPagedRich(context.Context, store.SessionListOpts) store.SessionListRichResult {
	return store.SessionListRichResult{}
}
func (s *localWorkerSessionStore) LastUsedChannel(context.Context, string) (string, string) {
	return "", ""
}

func setLoopStringField(t *testing.T, loop *Loop, fieldName, value string) {
	t.Helper()
	v := reflect.ValueOf(loop).Elem()
	f := v.FieldByName(fieldName)
	if !f.IsValid() {
		t.Fatalf("Loop field %q not found", fieldName)
	}
	if f.Kind() != reflect.String {
		t.Fatalf("Loop field %q is %s, want string", fieldName, f.Kind())
	}
	if !f.CanSet() {
		reflect.NewAt(f.Type(), unsafePointer(f)).Elem().SetString(value)
		return
	}
	f.SetString(value)
}

func setLoopDispatcherField(t *testing.T, loop *Loop, manager *localworker.Manager, workerStore store.WorkerStore) {
	t.Helper()
	v := reflect.ValueOf(loop).Elem()
	f := v.FieldByName("localWorkerDispatcher")
	if !f.IsValid() {
		t.Fatalf("Loop field %q not found", "localWorkerDispatcher")
	}
	if f.IsNil() {
		dispatcherType := f.Type()
		dispatcherValue := reflect.New(dispatcherType.Elem())
		if mf := dispatcherValue.Elem().FieldByName("manager"); mf.IsValid() {
			setReflectValue(t, mf, reflect.ValueOf(manager))
		}
		if sf := dispatcherValue.Elem().FieldByName("workers"); sf.IsValid() {
			setReflectValue(t, sf, reflect.ValueOf(workerStore))
		}
		setReflectValue(t, f, dispatcherValue)
		return
	}
	if mf := f.Elem().FieldByName("manager"); mf.IsValid() {
		setReflectValue(t, mf, reflect.ValueOf(manager))
	}
	if sf := f.Elem().FieldByName("workers"); sf.IsValid() {
		setReflectValue(t, sf, reflect.ValueOf(workerStore))
	}
}

func setReflectValue(t *testing.T, field reflect.Value, value reflect.Value) {
	t.Helper()
	if !value.Type().AssignableTo(field.Type()) {
		t.Fatalf("cannot assign %s to %s", value.Type(), field.Type())
	}
	if field.CanSet() {
		field.Set(value)
		return
	}
	reflect.NewAt(field.Type(), unsafePointer(field)).Elem().Set(value)
}

func unsafePointer(v reflect.Value) unsafe.Pointer {
	return unsafe.Pointer(v.UnsafeAddr())
}
