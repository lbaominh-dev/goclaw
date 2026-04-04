package methods

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

type updateCaptureStore struct {
	agent   *store.AgentData
	updates map[string]any
	updated bool
}

func (s *updateCaptureStore) Create(_ context.Context, _ *store.AgentData) error { return nil }
func (s *updateCaptureStore) GetByKey(_ context.Context, _ string) (*store.AgentData, error) {
	return s.agent, nil
}
func (s *updateCaptureStore) GetByID(_ context.Context, _ uuid.UUID) (*store.AgentData, error) {
	return nil, nil
}
func (s *updateCaptureStore) GetByIDUnscoped(_ context.Context, _ uuid.UUID) (*store.AgentData, error) {
	return nil, nil
}
func (s *updateCaptureStore) GetByKeys(_ context.Context, _ []string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *updateCaptureStore) GetByIDs(_ context.Context, _ []uuid.UUID) ([]store.AgentData, error) {
	return nil, nil
}
func (s *updateCaptureStore) Update(_ context.Context, _ uuid.UUID, updates map[string]any) error {
	s.updated = true
	s.updates = updates
	return nil
}
func (s *updateCaptureStore) Delete(_ context.Context, _ uuid.UUID) error { return nil }
func (s *updateCaptureStore) List(_ context.Context, _ string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *updateCaptureStore) GetDefault(_ context.Context) (*store.AgentData, error) {
	return nil, nil
}
func (s *updateCaptureStore) ShareAgent(_ context.Context, _ uuid.UUID, _, _, _ string) error {
	return nil
}
func (s *updateCaptureStore) RevokeShare(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (s *updateCaptureStore) ListShares(_ context.Context, _ uuid.UUID) ([]store.AgentShareData, error) {
	return nil, nil
}
func (s *updateCaptureStore) CanAccess(_ context.Context, _ uuid.UUID, _ string) (bool, string, error) {
	return true, "owner", nil
}
func (s *updateCaptureStore) ListAccessible(_ context.Context, _ string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *updateCaptureStore) GetAgentContextFiles(_ context.Context, _ uuid.UUID) ([]store.AgentContextFileData, error) {
	return nil, nil
}
func (s *updateCaptureStore) SetAgentContextFile(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}
func (s *updateCaptureStore) GetUserContextFiles(_ context.Context, _ uuid.UUID, _ string) ([]store.UserContextFileData, error) {
	return nil, nil
}
func (s *updateCaptureStore) SetUserContextFile(_ context.Context, _ uuid.UUID, _, _, _ string) error {
	return nil
}
func (s *updateCaptureStore) ListUserContextFilesByName(_ context.Context, _ uuid.UUID, _ string) ([]store.UserContextFileData, error) {
	return nil, nil
}
func (s *updateCaptureStore) DeleteUserContextFile(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}
func (s *updateCaptureStore) MigrateUserDataOnMerge(_ context.Context, _ []string, _ string) error {
	return nil
}
func (s *updateCaptureStore) GetUserOverride(_ context.Context, _ uuid.UUID, _ string) (*store.UserAgentOverrideData, error) {
	return nil, nil
}
func (s *updateCaptureStore) SetUserOverride(_ context.Context, _ *store.UserAgentOverrideData) error {
	return nil
}
func (s *updateCaptureStore) GetOrCreateUserProfile(_ context.Context, _ uuid.UUID, _, _, _ string) (bool, string, error) {
	return false, "", nil
}
func (s *updateCaptureStore) ListUserInstances(_ context.Context, _ uuid.UUID) ([]store.UserInstanceData, error) {
	return nil, nil
}
func (s *updateCaptureStore) UpdateUserProfileMetadata(_ context.Context, _ uuid.UUID, _ string, _ map[string]string) error {
	return nil
}
func (s *updateCaptureStore) EnsureUserProfile(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (s *updateCaptureStore) PropagateContextFile(_ context.Context, _ uuid.UUID, _ string) (int, error) {
	return 0, nil
}

func buildUpdateRequest(t *testing.T, params map[string]any) *protocol.RequestFrame {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return &protocol.RequestFrame{
		ID:     "test-update-req-1",
		Method: protocol.MethodAgentsUpdate,
		Params: raw,
	}
}

func TestHandleUpdate_PersistsLocalWorkerSettings(t *testing.T) {
	stub := &updateCaptureStore{agent: &store.AgentData{BaseModel: store.BaseModel{ID: uuid.New()}, AgentKey: "agent-1", ExecutionMode: store.AgentExecutionModeServer}}
	m := newManagedMethods(t, stub)

	req := buildUpdateRequest(t, map[string]any{
		"agentId":            "agent-1",
		"execution_mode":     store.AgentExecutionModeLocalWorker,
		"local_runtime_kind": "wails_desktop",
		"worker_endpoint_id": uuid.NewString(),
		"bound_worker_id":    "worker-456",
		"workspace_key":      "desktop-main",
	})

	m.handleUpdate(context.Background(), nullClient(), req)

	if !stub.updated {
		t.Fatal("agentStore.Update was not called")
	}
	if got := stub.updates["execution_mode"]; got != store.AgentExecutionModeLocalWorker {
		t.Fatalf("execution_mode = %#v, want %q", got, store.AgentExecutionModeLocalWorker)
	}
	if got := stub.updates["local_runtime_kind"]; got != "wails_desktop" {
		t.Fatalf("local_runtime_kind = %#v, want %q", got, "wails_desktop")
	}
	if got := stub.updates["worker_endpoint_id"]; got == nil || got == "" {
		t.Fatalf("worker_endpoint_id = %#v, want non-empty value", got)
	}
	if got := stub.updates["bound_worker_id"]; got != "worker-456" {
		t.Fatalf("bound_worker_id = %#v, want %q", got, "worker-456")
	}
	if got := stub.updates["workspace_key"]; got != "desktop-main" {
		t.Fatalf("workspace_key = %#v, want %q", got, "desktop-main")
	}
}

func TestHandleUpdate_RejectsOpencodeLocalWorkerWithoutWorkspaceKey(t *testing.T) {
	stub := &updateCaptureStore{agent: &store.AgentData{BaseModel: store.BaseModel{ID: uuid.New()}, AgentKey: "agent-1", ExecutionMode: store.AgentExecutionModeServer}}
	m := newManagedMethods(t, stub)
	client := responseClient()

	req := buildUpdateRequest(t, map[string]any{
		"agentId":            "agent-1",
		"execution_mode":     store.AgentExecutionModeLocalWorker,
		"local_runtime_kind": "opencode",
		"worker_endpoint_id": uuid.NewString(),
	})

	m.handleUpdate(context.Background(), client, req)

	if stub.updated {
		t.Fatal("agentStore.Update should not be called without workspace_key")
	}

	resp := readResponse(t, client)
	if resp.OK {
		t.Fatal("expected error response for missing workspace_key")
	}
	if resp.Error == nil {
		t.Fatal("expected error details in response")
	}
	if !strings.Contains(resp.Error.Message, "workspace_key") {
		t.Fatalf("error message = %q, want workspace_key validation failure", resp.Error.Message)
	}
}

func TestHandleUpdate_RejectsInvalidLocalWorkerConfig(t *testing.T) {
	stub := &updateCaptureStore{agent: &store.AgentData{BaseModel: store.BaseModel{ID: uuid.New()}, AgentKey: "agent-1", ExecutionMode: store.AgentExecutionModeServer}}
	m := newManagedMethods(t, stub)
	client := responseClient()

	req := buildUpdateRequest(t, map[string]any{
		"agentId":         "agent-1",
		"execution_mode":  store.AgentExecutionModeLocalWorker,
		"bound_worker_id": "worker-456",
	})

	m.handleUpdate(context.Background(), client, req)

	if stub.updated {
		t.Fatal("agentStore.Update should not be called for invalid local worker config")
	}

	resp := readResponse(t, client)
	if resp.OK {
		t.Fatal("expected error response for invalid local worker config")
	}
	if resp.Error == nil {
		t.Fatal("expected error details in response")
	}
	if resp.Error.Code != protocol.ErrInvalidRequest {
		t.Fatalf("error code = %q, want %q", resp.Error.Code, protocol.ErrInvalidRequest)
	}
	if !strings.Contains(resp.Error.Message, "local_runtime_kind and worker_endpoint_id") {
		t.Fatalf("error message = %q, want local worker validation failure", resp.Error.Message)
	}
}

func TestHandleUpdate_RejectsMalformedWorkerEndpointID(t *testing.T) {
	stub := &updateCaptureStore{agent: &store.AgentData{BaseModel: store.BaseModel{ID: uuid.New()}, AgentKey: "agent-1", ExecutionMode: store.AgentExecutionModeServer}}
	m := newManagedMethods(t, stub)
	client := responseClient()

	req := buildUpdateRequest(t, map[string]any{
		"agentId":            "agent-1",
		"execution_mode":     store.AgentExecutionModeLocalWorker,
		"local_runtime_kind": "wails_desktop",
		"worker_endpoint_id": "not-a-uuid",
	})

	m.handleUpdate(context.Background(), client, req)

	if stub.updated {
		t.Fatal("agentStore.Update should not be called for malformed worker endpoint id")
	}

	resp := readResponse(t, client)
	if resp.OK {
		t.Fatal("expected error response for malformed worker endpoint id")
	}
	if resp.Error == nil {
		t.Fatal("expected error details in response")
	}
	if resp.Error.Code != protocol.ErrInvalidRequest {
		t.Fatalf("error code = %q, want %q", resp.Error.Code, protocol.ErrInvalidRequest)
	}
	if !strings.Contains(resp.Error.Message, "worker_endpoint_id") {
		t.Fatalf("error message = %q, want worker_endpoint_id validation failure", resp.Error.Message)
	}
}
