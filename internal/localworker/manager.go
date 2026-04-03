package localworker

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

type registryKey struct {
	tenantID uuid.UUID
	workerID string
}

// Manager tracks currently connected local workers in memory.
type Manager struct {
	mu      sync.RWMutex
	workers map[registryKey]Registration
}

func NewManager() *Manager {
	return &Manager{
		workers: make(map[registryKey]Registration),
	}
}

func (m *Manager) Register(tenantID uuid.UUID, workerID string, conn Connection) (*Registration, error) {
	if conn == nil {
		return nil, ErrWorkerConnectionRequired
	}

	reg := Registration{
		TenantID:   tenantID,
		WorkerID:   workerID,
		Connection: conn,
	}

	m.mu.Lock()
	m.workers[registryKey{tenantID: tenantID, workerID: workerID}] = reg
	m.mu.Unlock()

	copy := reg
	return &copy, nil
}

func (m *Manager) Get(tenantID uuid.UUID, workerID string) (*Registration, bool) {
	m.mu.RLock()
	reg, ok := m.workers[registryKey{tenantID: tenantID, workerID: workerID}]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	copy := reg
	return &copy, true
}

func (m *Manager) IsOnline(tenantID uuid.UUID, workerID string) bool {
	_, ok := m.Get(tenantID, workerID)
	return ok
}

func (m *Manager) Disconnect(tenantID uuid.UUID, workerID string) bool {
	return m.disconnectIf(tenantID, workerID, nil)
}

func (m *Manager) DisconnectIfConnection(tenantID uuid.UUID, workerID string, conn Connection) bool {
	if conn == nil {
		return false
	}
	return m.disconnectIf(tenantID, workerID, func(reg Registration) bool {
		return reg.Connection == conn
	})
}

func (m *Manager) disconnectIf(tenantID uuid.UUID, workerID string, pred disconnectPredicate) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := registryKey{tenantID: tenantID, workerID: workerID}
	reg, ok := m.workers[key]
	if !ok {
		return false
	}
	if pred != nil && !pred(reg) {
		return false
	}
	delete(m.workers, key)
	return true
}

func (m *Manager) Dispatch(ctx context.Context, tenantID uuid.UUID, workerID string, env Envelope) error {
	m.mu.RLock()
	reg, ok := m.workers[registryKey{tenantID: tenantID, workerID: workerID}]
	if !ok {
		m.mu.RUnlock()
		return ErrWorkerNotConnected
	}
	env.TenantID = tenantID
	env.WorkerID = workerID
	err := reg.Connection.Dispatch(ctx, env)
	m.mu.RUnlock()
	return err
}
