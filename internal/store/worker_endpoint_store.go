package store

import (
	"context"
	"github.com/google/uuid"
)

// WorkerEndpointData represents a persisted outbound worker endpoint profile.
type WorkerEndpointData struct {
	BaseModel
	TenantID    uuid.UUID `json:"tenant_id"`
	Name        string    `json:"name"`
	RuntimeKind string    `json:"runtime_kind"`
	EndpointURL string    `json:"endpoint_url"`
	AuthToken   string    `json:"auth_token"`
}

// WorkerEndpointStore persists outbound worker endpoint profiles.
type WorkerEndpointStore interface {
	Create(ctx context.Context, endpoint *WorkerEndpointData) error
	Get(ctx context.Context, id uuid.UUID) (*WorkerEndpointData, error)
	List(ctx context.Context) ([]WorkerEndpointData, error)
	Update(ctx context.Context, id uuid.UUID, updates map[string]any) error
	Delete(ctx context.Context, id uuid.UUID) error
}
