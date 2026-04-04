package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type httpAgentUpdateStore struct {
	agent   *store.AgentData
	updates map[string]any
	updated bool
}

func (s *httpAgentUpdateStore) Create(context.Context, *store.AgentData) error { return nil }
func (s *httpAgentUpdateStore) GetByKey(context.Context, string) (*store.AgentData, error) {
	return nil, nil
}
func (s *httpAgentUpdateStore) GetByID(_ context.Context, _ uuid.UUID) (*store.AgentData, error) {
	return s.agent, nil
}
func (s *httpAgentUpdateStore) GetByIDUnscoped(context.Context, uuid.UUID) (*store.AgentData, error) {
	return nil, nil
}
func (s *httpAgentUpdateStore) GetByKeys(context.Context, []string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *httpAgentUpdateStore) GetByIDs(context.Context, []uuid.UUID) ([]store.AgentData, error) {
	return nil, nil
}
func (s *httpAgentUpdateStore) Update(_ context.Context, _ uuid.UUID, updates map[string]any) error {
	s.updated = true
	s.updates = updates
	return nil
}
func (s *httpAgentUpdateStore) Delete(context.Context, uuid.UUID) error { return nil }
func (s *httpAgentUpdateStore) List(context.Context, string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *httpAgentUpdateStore) GetDefault(context.Context) (*store.AgentData, error) { return nil, nil }
func (s *httpAgentUpdateStore) ShareAgent(context.Context, uuid.UUID, string, string, string) error {
	return nil
}
func (s *httpAgentUpdateStore) RevokeShare(context.Context, uuid.UUID, string) error { return nil }
func (s *httpAgentUpdateStore) ListShares(context.Context, uuid.UUID) ([]store.AgentShareData, error) {
	return nil, nil
}
func (s *httpAgentUpdateStore) CanAccess(context.Context, uuid.UUID, string) (bool, string, error) {
	return true, "owner", nil
}
func (s *httpAgentUpdateStore) ListAccessible(context.Context, string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *httpAgentUpdateStore) GetAgentContextFiles(context.Context, uuid.UUID) ([]store.AgentContextFileData, error) {
	return nil, nil
}
func (s *httpAgentUpdateStore) SetAgentContextFile(context.Context, uuid.UUID, string, string) error {
	return nil
}
func (s *httpAgentUpdateStore) GetUserContextFiles(context.Context, uuid.UUID, string) ([]store.UserContextFileData, error) {
	return nil, nil
}
func (s *httpAgentUpdateStore) SetUserContextFile(context.Context, uuid.UUID, string, string, string) error {
	return nil
}
func (s *httpAgentUpdateStore) ListUserContextFilesByName(context.Context, uuid.UUID, string) ([]store.UserContextFileData, error) {
	return nil, nil
}
func (s *httpAgentUpdateStore) DeleteUserContextFile(context.Context, uuid.UUID, string, string) error {
	return nil
}
func (s *httpAgentUpdateStore) MigrateUserDataOnMerge(context.Context, []string, string) error {
	return nil
}
func (s *httpAgentUpdateStore) GetUserOverride(context.Context, uuid.UUID, string) (*store.UserAgentOverrideData, error) {
	return nil, nil
}
func (s *httpAgentUpdateStore) SetUserOverride(context.Context, *store.UserAgentOverrideData) error {
	return nil
}
func (s *httpAgentUpdateStore) GetOrCreateUserProfile(context.Context, uuid.UUID, string, string, string) (bool, string, error) {
	return false, "", nil
}
func (s *httpAgentUpdateStore) ListUserInstances(context.Context, uuid.UUID) ([]store.UserInstanceData, error) {
	return nil, nil
}
func (s *httpAgentUpdateStore) UpdateUserProfileMetadata(context.Context, uuid.UUID, string, map[string]string) error {
	return nil
}
func (s *httpAgentUpdateStore) EnsureUserProfile(context.Context, uuid.UUID, string) error {
	return nil
}
func (s *httpAgentUpdateStore) PropagateContextFile(context.Context, uuid.UUID, string) (int, error) {
	return 0, nil
}

func TestHTTPAgentUpdatePersistsWorkerEndpointID(t *testing.T) {
	agentID := uuid.New()
	endpointID := uuid.NewString()
	stub := &httpAgentUpdateStore{agent: &store.AgentData{
		BaseModel:        store.BaseModel{ID: agentID},
		OwnerID:          "user-1",
		AgentKey:         "agent-1",
		ExecutionMode:    store.AgentExecutionModeServer,
		Provider:         "openai",
		LocalRuntimeKind: "",
		WorkerEndpointID: "",
		BoundWorkerID:    "",
	}}
	handler := &AgentsHandler{agents: stub}

	body, err := json.Marshal(map[string]any{
		"execution_mode":     store.AgentExecutionModeLocalWorker,
		"local_runtime_kind": "wails_desktop",
		"worker_endpoint_id": endpointID,
		"bound_worker_id":    "worker-123",
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest("PUT", "/v1/agents/"+agentID.String(), bytes.NewReader(body))
	req.SetPathValue("id", agentID.String())
	ctx := store.WithUserID(context.Background(), "user-1")
	ctx = store.WithTenantID(ctx, store.MasterTenantID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.handleUpdate(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if !stub.updated {
		t.Fatal("agent store Update was not called")
	}
	if got, ok := stub.updates["local_runtime_kind"].(string); !ok || got != "wails_desktop" {
		t.Fatalf("local_runtime_kind = %#v, want string %q", stub.updates["local_runtime_kind"], "wails_desktop")
	}
	if got, ok := stub.updates["worker_endpoint_id"].(string); !ok || got != endpointID {
		t.Fatalf("worker_endpoint_id = %#v, want string %q", stub.updates["worker_endpoint_id"], endpointID)
	}
	if got, ok := stub.updates["bound_worker_id"].(string); !ok || got != "worker-123" {
		t.Fatalf("bound_worker_id = %#v, want string %q", stub.updates["bound_worker_id"], "worker-123")
	}
}
