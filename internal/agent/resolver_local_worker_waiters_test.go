package agent

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/localworker"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestManagedResolver_LocalWorkerAgentGetsSharedWaiterRegistry(t *testing.T) {
	tenantID := uuid.New()
	agentID := uuid.New()
	waiters := localworker.NewWaiterRegistry()
	dispatcher := &localworker.Dispatcher{}
	providerReg := providers.NewRegistry(store.TenantIDFromContext)
	providerReg.RegisterForTenant(tenantID, &resolverWaiterTestProvider{name: "test-provider"})

	resolver := NewManagedResolver(ResolverDeps{
		AgentStore: resolverWaiterTestAgentStore{agent: &store.AgentData{
			BaseModel:        store.BaseModel{ID: agentID},
			TenantID:         tenantID,
			AgentKey:         "local-worker-agent",
			Provider:         "test-provider",
			Model:            "test-model",
			Status:           store.AgentStatusActive,
			AgentType:        store.AgentTypeOpen,
			ExecutionMode:    store.AgentExecutionModeLocalWorker,
			LocalRuntimeKind: "wails_desktop",
			WorkerEndpointID: uuid.NewString(),
		}},
		ProviderReg:           providerReg,
		LocalWorkerDispatcher: dispatcher,
		LocalWorkerWaiters:    waiters,
	})

	resolved, err := resolver(store.WithTenantID(context.Background(), tenantID), "local-worker-agent")
	if err != nil {
		t.Fatalf("resolver() error = %v", err)
	}

	loop, ok := resolved.(*Loop)
	if !ok {
		t.Fatalf("resolved agent type = %T, want *Loop", resolved)
	}
	if loop.localWorkerWaiters != waiters {
		t.Fatalf("loop waiter registry = %p, want shared %p", loop.localWorkerWaiters, waiters)
	}
	if loop.localWorkerDispatcher != dispatcher {
		t.Fatalf("loop dispatcher = %p, want shared %p", loop.localWorkerDispatcher, dispatcher)
	}
}

type resolverWaiterTestAgentStore struct {
	agent *store.AgentData
}

func (s resolverWaiterTestAgentStore) Create(context.Context, *store.AgentData) error {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) GetByKey(_ context.Context, agentKey string) (*store.AgentData, error) {
	if s.agent != nil && s.agent.AgentKey == agentKey {
		return s.agent, nil
	}
	return nil, nil
}
func (s resolverWaiterTestAgentStore) GetByID(_ context.Context, id uuid.UUID) (*store.AgentData, error) {
	if s.agent != nil && s.agent.ID == id {
		return s.agent, nil
	}
	return nil, nil
}
func (s resolverWaiterTestAgentStore) GetByIDUnscoped(context.Context, uuid.UUID) (*store.AgentData, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) GetByKeys(context.Context, []string) ([]store.AgentData, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) GetByIDs(context.Context, []uuid.UUID) ([]store.AgentData, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) Update(context.Context, uuid.UUID, map[string]any) error {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) Delete(context.Context, uuid.UUID) error { panic("not used") }
func (s resolverWaiterTestAgentStore) List(context.Context, string) ([]store.AgentData, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) GetDefault(context.Context) (*store.AgentData, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) ShareAgent(context.Context, uuid.UUID, string, string, string) error {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) RevokeShare(context.Context, uuid.UUID, string) error {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) ListShares(context.Context, uuid.UUID) ([]store.AgentShareData, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) CanAccess(context.Context, uuid.UUID, string) (bool, string, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) ListAccessible(context.Context, string) ([]store.AgentData, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) GetAgentContextFiles(context.Context, uuid.UUID) ([]store.AgentContextFileData, error) {
	return nil, nil
}
func (s resolverWaiterTestAgentStore) SetAgentContextFile(context.Context, uuid.UUID, string, string) error {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) PropagateContextFile(context.Context, uuid.UUID, string) (int, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) GetUserContextFiles(context.Context, uuid.UUID, string) ([]store.UserContextFileData, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) ListUserContextFilesByName(context.Context, uuid.UUID, string) ([]store.UserContextFileData, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) SetUserContextFile(context.Context, uuid.UUID, string, string, string) error {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) DeleteUserContextFile(context.Context, uuid.UUID, string, string) error {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) MigrateUserDataOnMerge(context.Context, []string, string) error {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) GetUserOverride(context.Context, uuid.UUID, string) (*store.UserAgentOverrideData, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) SetUserOverride(context.Context, *store.UserAgentOverrideData) error {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) GetOrCreateUserProfile(context.Context, uuid.UUID, string, string, string) (bool, string, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) EnsureUserProfile(context.Context, uuid.UUID, string) error {
	return nil
}
func (s resolverWaiterTestAgentStore) ListUserInstances(context.Context, uuid.UUID) ([]store.UserInstanceData, error) {
	panic("not used")
}
func (s resolverWaiterTestAgentStore) UpdateUserProfileMetadata(context.Context, uuid.UUID, string, map[string]string) error {
	panic("not used")
}

type resolverWaiterTestProvider struct{ name string }

func (p *resolverWaiterTestProvider) Chat(context.Context, providers.ChatRequest) (*providers.ChatResponse, error) {
	panic("not used")
}

func (p *resolverWaiterTestProvider) ChatStream(context.Context, providers.ChatRequest, func(providers.StreamChunk)) (*providers.ChatResponse, error) {
	panic("not used")
}

func (p *resolverWaiterTestProvider) DefaultModel() string { return "test-model" }

func (p *resolverWaiterTestProvider) Name() string { return p.name }
