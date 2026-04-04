package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/nextlevelbuilder/goclaw/internal/localworker"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestRunRequest_LocalWorkerMember_DispatchesJobInsteadOfProvider(t *testing.T) {
	tenantID := uuid.New()
	agentID := uuid.New()
	provider := &localWorkerTestProvider{}
	workerStore := &localWorkerTestStore{}
	endpointID := uuid.New()
	endpointServer := newLocalWorkerOutboundTestServer(t, "Bearer endpoint-token")
	endpointStore := &localWorkerTestEndpointStore{
		endpoint: &store.WorkerEndpointData{
			BaseModel:   store.BaseModel{ID: endpointID},
			TenantID:    tenantID,
			Name:        "worker-123",
			EndpointURL: localWorkerHTTPToWebsocketURL(endpointServer.server.URL),
			AuthToken:   "Bearer endpoint-token",
		},
	}
	outbound := localworker.NewOutboundManager(endpointStore)

	loop := newLocalWorkerTestLoop(t, provider, tenantID, agentID)
	setLoopStringField(t, loop, "executionMode", store.AgentExecutionModeLocalWorker)
	setLoopStringField(t, loop, "localRuntimeKind", "wails_desktop")
	setLoopStringField(t, loop, "boundWorkerID", "worker-123")
	setLoopStringField(t, loop, "workerEndpointID", endpointID.String())
	setLoopDispatcherField(t, loop, outbound, workerStore)

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
	if job.WorkerID != endpointID.String() {
		t.Fatalf("job worker_id = %q, want %q", job.WorkerID, endpointID.String())
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
	endpointServer.waitForMessages(t, 1)
	if endpointServer.messageCount() != 1 {
		t.Fatalf("dispatch envelopes = %d, want 1", endpointServer.messageCount())
	}
	var dispatchEnvelope localworker.OutboundEnvelope
	if err := json.Unmarshal(endpointServer.message(0), &dispatchEnvelope); err != nil {
		t.Fatalf("unmarshal dispatch envelope: %v", err)
	}
	if dispatchEnvelope.Type != localworker.OutboundEnvelopeJobDispatch {
		t.Fatalf("dispatch type = %q, want %q", dispatchEnvelope.Type, localworker.OutboundEnvelopeJobDispatch)
	}
	dispatchPayloadBytes, err := json.Marshal(dispatchEnvelope.Payload)
	if err != nil {
		t.Fatalf("marshal dispatch payload: %v", err)
	}
	var dispatchPayload localworker.OutboundJobDispatch
	if err := json.Unmarshal(dispatchPayloadBytes, &dispatchPayload); err != nil {
		t.Fatalf("unmarshal dispatch payload: %v", err)
	}
	if dispatchPayload.JobID != job.ID.String() {
		t.Fatalf("payload jobId = %q, want %q", dispatchPayload.JobID, job.ID.String())
	}
	if dispatchPayload.RuntimeKind != "wails_desktop" {
		t.Fatalf("payload runtimeKind = %q, want %q", dispatchPayload.RuntimeKind, "wails_desktop")
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
	var dispatchedPayload localWorkerDispatchPayload
	if err := json.Unmarshal(dispatchPayload.Job, &dispatchedPayload); err != nil {
		t.Fatalf("unmarshal dispatched job payload: %v", err)
	}
	if dispatchedPayload.RunID != persistedPayload.RunID ||
		dispatchedPayload.SessionKey != persistedPayload.SessionKey ||
		dispatchedPayload.AgentID != persistedPayload.AgentID ||
		dispatchedPayload.AgentKey != persistedPayload.AgentKey ||
		dispatchedPayload.Message != persistedPayload.Message {
		t.Fatalf("dispatched payload = %+v, want semantic match with persisted payload %+v", dispatchedPayload, persistedPayload)
	}
}

func TestRunRequest_LocalWorkerMember_UsesWorkerEndpointID(t *testing.T) {
	tenantID := uuid.New()
	agentID := uuid.New()
	provider := &localWorkerTestProvider{}
	workerStore := &localWorkerTestStore{}
	endpointID := uuid.New()
	endpointServer := newLocalWorkerOutboundTestServer(t, "Bearer endpoint-token")
	endpointStore := &localWorkerTestEndpointStore{
		endpoint: &store.WorkerEndpointData{
			BaseModel:   store.BaseModel{ID: endpointID},
			TenantID:    tenantID,
			Name:        "worker endpoint",
			EndpointURL: localWorkerHTTPToWebsocketURL(endpointServer.server.URL),
			AuthToken:   "Bearer endpoint-token",
		},
	}
	outbound := localworker.NewOutboundManager(endpointStore)

	loop := newLocalWorkerTestLoop(t, provider, tenantID, agentID)
	setLoopStringField(t, loop, "executionMode", store.AgentExecutionModeLocalWorker)
	setLoopStringField(t, loop, "localRuntimeKind", "wails_desktop")
	setLoopStringField(t, loop, "boundWorkerID", "legacy-worker-id")
	setLoopStringField(t, loop, "workerEndpointID", endpointID.String())
	setLoopDispatcherField(t, loop, outbound, workerStore)

	result, err := loop.Run(store.WithTenantID(context.Background(), tenantID), RunRequest{
		SessionKey:  "agent:test:ws:direct:chat-endpoint",
		Message:     "Use endpoint binding",
		Channel:     "ws",
		ChannelType: "web",
		ChatID:      "chat-endpoint",
		PeerKind:    "direct",
		RunID:       "run-endpoint-binding",
		UserID:      "user-endpoint",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result == nil || !result.Queued {
		t.Fatalf("Run result = %+v, want queued local worker result", result)
	}
	if len(workerStore.jobs) != 1 {
		t.Fatalf("created jobs = %d, want 1", len(workerStore.jobs))
	}
	if workerStore.jobs[0].WorkerID != endpointID.String() {
		t.Fatalf("job worker_id = %q, want endpoint id %q", workerStore.jobs[0].WorkerID, endpointID.String())
	}
	endpointServer.waitForMessages(t, 1)
	if endpointServer.messageCount() != 1 {
		t.Fatalf("dispatch envelopes = %d, want 1", endpointServer.messageCount())
	}
	var dispatchEnvelope localworker.OutboundEnvelope
	if err := json.Unmarshal(endpointServer.message(0), &dispatchEnvelope); err != nil {
		t.Fatalf("unmarshal dispatch envelope: %v", err)
	}
	if dispatchEnvelope.Type != localworker.OutboundEnvelopeJobDispatch {
		t.Fatalf("dispatch type = %q, want %q", dispatchEnvelope.Type, localworker.OutboundEnvelopeJobDispatch)
	}
	if provider.chatCalls != 0 || provider.chatStreamCalls != 0 {
		t.Fatalf("provider should not be used, got chat=%d stream=%d", provider.chatCalls, provider.chatStreamCalls)
	}
}

func TestRunRequest_LocalWorkerMember_RejectsMissingEndpointBinding(t *testing.T) {
	tenantID := uuid.New()
	provider := &localWorkerTestProvider{}
	workerStore := &localWorkerTestStore{}

	loop := newLocalWorkerTestLoop(t, provider, tenantID, uuid.New())
	setLoopStringField(t, loop, "executionMode", store.AgentExecutionModeLocalWorker)
	setLoopStringField(t, loop, "localRuntimeKind", "wails_desktop")
	setLoopStringField(t, loop, "boundWorkerID", "legacy-worker-id")
	setLoopStringField(t, loop, "workerEndpointID", "")
	setLoopDispatcherField(t, loop, localworker.NewOutboundManager(&localWorkerTestEndpointStore{}), workerStore)

	_, err := loop.Run(store.WithTenantID(context.Background(), tenantID), RunRequest{
		SessionKey:  "agent:test:ws:direct:chat-missing-endpoint",
		Message:     "Missing endpoint binding",
		Channel:     "ws",
		ChannelType: "web",
		ChatID:      "chat-missing-endpoint",
		PeerKind:    "direct",
		RunID:       "run-missing-endpoint",
		UserID:      "user-endpoint",
	})
	if err == nil {
		t.Fatal("expected missing endpoint binding error")
	}
	if !strings.Contains(err.Error(), "worker endpoint") {
		t.Fatalf("error = %v, want missing endpoint binding error", err)
	}
	if len(workerStore.jobs) != 0 {
		t.Fatalf("created jobs = %d, want 0", len(workerStore.jobs))
	}
}

func TestRunRequest_LocalWorkerMember_DispatchesThroughOutboundManager(t *testing.T) {
	tenantID := uuid.New()
	agentID := uuid.New()
	provider := &localWorkerTestProvider{}
	workerStore := &localWorkerTestStore{}
	endpointID := uuid.New()
	endpointServer := newLocalWorkerOutboundTestServer(t, "Bearer endpoint-token")
	endpointStore := &localWorkerTestEndpointStore{
		endpoint: &store.WorkerEndpointData{
			BaseModel:   store.BaseModel{ID: endpointID},
			TenantID:    tenantID,
			Name:        "outbound-endpoint",
			EndpointURL: localWorkerHTTPToWebsocketURL(endpointServer.server.URL),
			AuthToken:   "Bearer endpoint-token",
		},
	}
	outbound := localworker.NewOutboundManager(endpointStore)

	loop := newLocalWorkerTestLoop(t, provider, tenantID, agentID)
	setLoopStringField(t, loop, "executionMode", store.AgentExecutionModeLocalWorker)
	setLoopStringField(t, loop, "localRuntimeKind", "wails_desktop")
	setLoopStringField(t, loop, "boundWorkerID", "worker-offline")
	setLoopStringField(t, loop, "workerEndpointID", endpointID.String())
	setLoopDispatcherField(t, loop, outbound, workerStore)

	result, err := loop.Run(store.WithTenantID(context.Background(), tenantID), RunRequest{
		SessionKey:  "agent:test:ws:direct:chat-2",
		Message:     "Run this on my laptop",
		Channel:     "ws",
		ChannelType: "web",
		ChatID:      "chat-2",
		PeerKind:    "direct",
		RunID:       "run-outbound-dispatch",
		UserID:      "user-999",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result == nil || !result.Queued {
		t.Fatalf("Run result = %+v, want queued local worker result", result)
	}
	if provider.chatCalls != 0 || provider.chatStreamCalls != 0 {
		t.Fatalf("provider should not be used, got chat=%d stream=%d", provider.chatCalls, provider.chatStreamCalls)
	}
	if len(workerStore.jobs) != 1 {
		t.Fatalf("created jobs = %d, want 1", len(workerStore.jobs))
	}
	endpointServer.waitForMessages(t, 1)
	if endpointStore.getCalls == 0 {
		t.Fatal("expected outbound manager to resolve endpoint before dispatch")
	}
}

func TestRunRequest_LocalWorkerMember_MarksJobFailedWhenDispatchErrors(t *testing.T) {
	tenantID := uuid.New()
	agentID := uuid.New()
	provider := &localWorkerTestProvider{}
	workerStore := &localWorkerTestStore{}
	endpointID := uuid.New()
	endpointServer := newLocalWorkerOutboundTestServer(t, "Bearer required-token")
	endpointStore := &localWorkerTestEndpointStore{
		endpoint: &store.WorkerEndpointData{
			BaseModel:   store.BaseModel{ID: endpointID},
			TenantID:    tenantID,
			Name:        "worker-123",
			EndpointURL: localWorkerHTTPToWebsocketURL(endpointServer.server.URL),
			AuthToken:   "Bearer wrong-token",
		},
	}
	outbound := localworker.NewOutboundManager(endpointStore)

	loop := newLocalWorkerTestLoop(t, provider, tenantID, agentID)
	setLoopStringField(t, loop, "executionMode", store.AgentExecutionModeLocalWorker)
	setLoopStringField(t, loop, "localRuntimeKind", "wails_desktop")
	setLoopStringField(t, loop, "boundWorkerID", "worker-123")
	setLoopStringField(t, loop, "workerEndpointID", endpointID.String())
	setLoopDispatcherField(t, loop, outbound, workerStore)

	ctx := store.WithTenantID(context.Background(), tenantID)
	_, err := loop.Run(ctx, RunRequest{
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
	if !strings.Contains(err.Error(), "401 Unauthorized") {
		t.Fatalf("error = %v, want outbound auth failure", err)
	}
	if len(workerStore.jobs) != 1 {
		t.Fatalf("created jobs = %d, want 1", len(workerStore.jobs))
	}
	job := workerStore.jobs[0]
	if job.Status != store.WorkerJobStatusFailed {
		t.Fatalf("job status = %q, want %q", job.Status, store.WorkerJobStatusFailed)
	}
	if !strings.Contains(string(job.Result), "401 Unauthorized") {
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

type localWorkerTestEndpointStore struct {
	endpoint *store.WorkerEndpointData
	getErr   error
	getCalls int
}

func (s *localWorkerTestEndpointStore) Create(context.Context, *store.WorkerEndpointData) error {
	return nil
}

func (s *localWorkerTestEndpointStore) Get(_ context.Context, id uuid.UUID) (*store.WorkerEndpointData, error) {
	s.getCalls++
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.endpoint == nil || s.endpoint.ID != id {
		return nil, nil
	}
	copy := *s.endpoint
	return &copy, nil
}

func (s *localWorkerTestEndpointStore) List(context.Context) ([]store.WorkerEndpointData, error) {
	return nil, nil
}

func (s *localWorkerTestEndpointStore) Update(context.Context, uuid.UUID, map[string]any) error {
	return nil
}

func (s *localWorkerTestEndpointStore) Delete(context.Context, uuid.UUID) error {
	return nil
}

type localWorkerOutboundTestServer struct {
	t *testing.T

	server             *httptest.Server
	requiredToken      string
	closeAfterNextRead bool

	mu         sync.Mutex
	messages   [][]byte
	messageCh  chan struct{}
	upgradeErr error
}

func newLocalWorkerOutboundTestServer(t *testing.T, requiredToken string) *localWorkerOutboundTestServer {
	t.Helper()

	ts := &localWorkerOutboundTestServer{
		t:             t,
		requiredToken: requiredToken,
		messageCh:     make(chan struct{}, 8),
	}
	upgrader := websocket.Upgrader{}
	ts.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token := r.Header.Get(localworker.DefaultOutboundAuthHeader); token != ts.requiredToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			ts.mu.Lock()
			ts.upgradeErr = err
			ts.mu.Unlock()
			return
		}

		go func() {
			defer conn.Close()
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					return
				}
				ts.mu.Lock()
				ts.messages = append(ts.messages, append([]byte(nil), msg...))
				closeAfterNextRead := ts.closeAfterNextRead
				if closeAfterNextRead {
					ts.closeAfterNextRead = false
				}
				ts.mu.Unlock()
				ts.messageCh <- struct{}{}
				if closeAfterNextRead {
					_ = conn.Close()
					return
				}
			}
		}()
	}))
	t.Cleanup(func() {
		ts.server.Close()
	})
	return ts
}

func (s *localWorkerOutboundTestServer) closeAfterFirstMessage() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeAfterNextRead = true
}

func (s *localWorkerOutboundTestServer) waitForMessages(t *testing.T, want int) {
	t.Helper()
	for i := 0; i < want; i++ {
		select {
		case <-s.messageCh:
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for message %d of %d", i+1, want)
		}
	}
}

func (s *localWorkerOutboundTestServer) message(index int) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.upgradeErr != nil {
		s.t.Fatalf("upgrade error: %v", s.upgradeErr)
	}
	if index >= len(s.messages) {
		s.t.Fatalf("message index %d out of range %d", index, len(s.messages))
	}
	return append([]byte(nil), s.messages[index]...)
}

func (s *localWorkerOutboundTestServer) messageCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

func localWorkerHTTPToWebsocketURL(raw string) string {
	if strings.HasPrefix(raw, "https://") {
		return "wss://" + strings.TrimPrefix(raw, "https://")
	}
	if strings.HasPrefix(raw, "http://") {
		return "ws://" + strings.TrimPrefix(raw, "http://")
	}
	return raw
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

func setLoopDispatcherField(t *testing.T, loop *Loop, transport any, workerStore store.WorkerStore) {
	t.Helper()
	v := reflect.ValueOf(loop).Elem()
	f := v.FieldByName("localWorkerDispatcher")
	if !f.IsValid() {
		t.Fatalf("Loop field %q not found", "localWorkerDispatcher")
	}
	if f.IsNil() {
		dispatcherType := f.Type()
		dispatcherValue := reflect.New(dispatcherType.Elem())
		setLocalWorkerDispatcherTransport(t, dispatcherValue.Elem(), transport)
		if sf := dispatcherValue.Elem().FieldByName("workers"); sf.IsValid() {
			setReflectValue(t, sf, reflect.ValueOf(workerStore))
		}
		setReflectValue(t, f, dispatcherValue)
		return
	}
	setLocalWorkerDispatcherTransport(t, f.Elem(), transport)
	if sf := f.Elem().FieldByName("workers"); sf.IsValid() {
		setReflectValue(t, sf, reflect.ValueOf(workerStore))
	}
}

func setLocalWorkerDispatcherTransport(t *testing.T, dispatcher reflect.Value, transport any) {
	t.Helper()
	if transport == nil {
		return
	}
	value := reflect.ValueOf(transport)
	for _, fieldName := range []string{"manager", "outbound", "outboundManager"} {
		field := dispatcher.FieldByName(fieldName)
		if !field.IsValid() || !value.Type().AssignableTo(field.Type()) {
			continue
		}
		setReflectValue(t, field, value)
		return
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
