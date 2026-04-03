package localworker

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrWorkerNotConnected       = errors.New("local worker not connected")
	ErrWorkerConnectionRequired = errors.New("local worker connection required")
)

// Connection is the live worker transport used by the in-memory registry.
// Later tasks can adapt gateway.Client to this shape.
type Connection interface {
	Dispatch(ctx context.Context, env Envelope) error
}

// Envelope is the minimal dispatch payload forwarded to a worker connection.
type Envelope struct {
	Type     string    `json:"type"`
	TenantID uuid.UUID `json:"tenantId"`
	WorkerID string    `json:"workerId"`
	Payload  any       `json:"payload,omitempty"`
}

// Registration represents one active worker connection in memory.
type Registration struct {
	TenantID   uuid.UUID
	WorkerID   string
	Connection Connection
}

type disconnectPredicate func(Registration) bool
