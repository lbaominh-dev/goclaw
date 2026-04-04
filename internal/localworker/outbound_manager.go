package localworker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type outboundClientKey struct {
	tenantID   uuid.UUID
	endpointID uuid.UUID
}

// OutboundManager maintains live outbound worker sockets keyed by endpoint profile ID.
type OutboundManager struct {
	mu        sync.Mutex
	endpoints store.WorkerEndpointStore
	dialer    websocketDialer
	clients   map[outboundClientKey]*outboundClient
	replies   OutboundReplyHandler
}

type OutboundReplyHandler interface {
	HandleOutboundWorkerMessage(ctx context.Context, endpointID uuid.UUID, message WorkerReplyEnvelope) error
}

func NewOutboundManager(endpoints store.WorkerEndpointStore) *OutboundManager {
	return &OutboundManager{
		endpoints: endpoints,
		dialer:    websocket.DefaultDialer,
		clients:   make(map[outboundClientKey]*outboundClient),
	}
}

func (m *OutboundManager) SetReplyHandler(handler OutboundReplyHandler) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.replies = handler
}

func (m *OutboundManager) Dispatch(ctx context.Context, endpointID uuid.UUID, env OutboundEnvelope) error {
	if m == nil || m.endpoints == nil {
		return fmt.Errorf("outbound worker manager not configured")
	}

	client, err := m.clientForDispatch(ctx, endpointID)
	if err != nil {
		return err
	}
	if err := client.Dispatch(ctx, env); err != nil {
		m.dropClient(cacheKeyFromContext(ctx, endpointID), client)
		return err
	}
	return nil
}

func (m *OutboundManager) clientForDispatch(ctx context.Context, endpointID uuid.UUID) (*outboundClient, error) {
	cacheKey := cacheKeyFromContext(ctx, endpointID)
	m.mu.Lock()
	client := m.clients[cacheKey]
	m.mu.Unlock()
	if client != nil && client.Healthy() {
		return client, nil
	}

	endpoint, err := m.endpoints.Get(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	if endpoint == nil {
		return nil, fmt.Errorf("worker endpoint %s not found", endpointID)
	}

	client, err = newOutboundClient(ctx, m.dialer, endpoint)
	if err != nil {
		return nil, err
	}
	m.startReplyListener(cacheKey, client)

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing := m.clients[cacheKey]; existing != nil && existing.Healthy() {
		_ = client.Close()
		return existing, nil
	}
	m.clients[cacheKey] = client
	return client, nil
}

func (m *OutboundManager) startReplyListener(key outboundClientKey, client *outboundClient) {
	if m == nil || client == nil {
		return
	}
	go m.listenReplies(key, client)
}

func (m *OutboundManager) listenReplies(key outboundClientKey, client *outboundClient) {
	for {
		msg, err := client.ReadReply()
		if err != nil {
			m.dropClient(key, client)
			return
		}
		if err := m.handleReplyMessage(key, msg); err != nil {
			slog.Warn("localworker.outbound.reply_handler_failed", "tenant_id", key.tenantID, "endpoint_id", key.endpointID, "err", err)
		}
	}
}

func (m *OutboundManager) handleReplyMessage(key outboundClientKey, msg []byte) error {
	var reply WorkerReplyEnvelope
	if err := json.Unmarshal(msg, &reply); err != nil {
		return fmt.Errorf("decode worker reply: %w", err)
	}
	m.mu.Lock()
	handler := m.replies
	m.mu.Unlock()
	if handler == nil {
		return nil
	}
	ctx := store.WithTenantID(context.Background(), key.tenantID)
	return handler.HandleOutboundWorkerMessage(ctx, key.endpointID, reply)
}

func (m *OutboundManager) dropClient(key outboundClientKey, client *outboundClient) {
	if client != nil {
		_ = client.Close()
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing := m.clients[key]; existing == client {
		delete(m.clients, key)
	}
}

func cacheKeyFromContext(ctx context.Context, endpointID uuid.UUID) outboundClientKey {
	return outboundClientKey{
		tenantID:   store.TenantIDFromContext(ctx),
		endpointID: endpointID,
	}
}
