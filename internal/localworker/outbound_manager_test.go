package localworker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestOutboundManager_ConnectsWithConfiguredHeaderAndReusesSocket(t *testing.T) {
	tenantID := uuid.New()
	endpointID := uuid.New()
	server := newOutboundWSTestServer(t, "token-123")

	endpointStore := &stubWorkerEndpointStore{
		endpoint: &store.WorkerEndpointData{
			BaseModel:   store.BaseModel{ID: endpointID},
			TenantID:    tenantID,
			Name:        "desktop-primary",
			RuntimeKind: "wails_desktop",
			EndpointURL: httpToWebsocketURL(server.server.URL),
			AuthToken:   "token-123",
		},
	}
	mgr := NewOutboundManager(endpointStore)
	ctx := store.WithTenantID(context.Background(), tenantID)

	if err := mgr.Dispatch(ctx, endpointID, OutboundEnvelope{Type: OutboundEnvelopeJobDispatch, Payload: map[string]any{"jobId": "job-1"}}); err != nil {
		t.Fatalf("first Dispatch error: %v", err)
	}
	if err := mgr.Dispatch(ctx, endpointID, OutboundEnvelope{Type: OutboundEnvelopeJobDispatch, Payload: map[string]any{"jobId": "job-2"}}); err != nil {
		t.Fatalf("second Dispatch error: %v", err)
	}

	server.waitForMessages(t, 2)
	if got := server.connectCount(); got != 1 {
		t.Fatalf("connect count = %d, want 1", got)
	}
	if got := endpointStore.getCalls; got != 1 {
		t.Fatalf("endpoint store Get calls = %d, want 1", got)
	}
	if got := server.headerValues(); len(got) != 1 || got[0] != "token-123" {
		t.Fatalf("Authorization headers = %#v, want [token-123]", got)
	}
}

func TestOutboundManager_FailsOnAuthReject(t *testing.T) {
	tenantID := uuid.New()
	endpointID := uuid.New()
	server := newOutboundWSTestServer(t, "expected-token")

	endpointStore := &stubWorkerEndpointStore{
		endpoint: &store.WorkerEndpointData{
			BaseModel:   store.BaseModel{ID: endpointID},
			TenantID:    tenantID,
			Name:        "desktop-primary",
			RuntimeKind: "wails_desktop",
			EndpointURL: httpToWebsocketURL(server.server.URL),
			AuthToken:   "wrong-token",
		},
	}
	mgr := NewOutboundManager(endpointStore)
	ctx := store.WithTenantID(context.Background(), tenantID)

	err := mgr.Dispatch(ctx, endpointID, OutboundEnvelope{Type: OutboundEnvelopeJobDispatch, Payload: map[string]any{"jobId": "job-1"}})
	if err == nil {
		t.Fatal("Dispatch error = nil, want auth rejection")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("Dispatch error = %q, want 401 handshake failure", err)
	}
	if got := server.connectCount(); got != 0 {
		t.Fatalf("connect count = %d, want 0 successful upgrades", got)
	}
}

func TestOutboundManager_DispatchesEnvelope(t *testing.T) {
	tenantID := uuid.New()
	endpointID := uuid.New()
	server := newOutboundWSTestServer(t, "token-123")

	endpointStore := &stubWorkerEndpointStore{
		endpoint: &store.WorkerEndpointData{
			BaseModel:   store.BaseModel{ID: endpointID},
			TenantID:    tenantID,
			Name:        "desktop-primary",
			RuntimeKind: "wails_desktop",
			EndpointURL: httpToWebsocketURL(server.server.URL),
			AuthToken:   "token-123",
		},
	}
	mgr := NewOutboundManager(endpointStore)
	ctx := store.WithTenantID(context.Background(), tenantID)
	want := OutboundEnvelope{
		Type: OutboundEnvelopeJobDispatch,
		Payload: map[string]any{
			"jobId":       "job-123",
			"runtimeKind": "wails_desktop",
		},
	}

	if err := mgr.Dispatch(ctx, endpointID, want); err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}

	server.waitForMessages(t, 1)
	var got OutboundEnvelope
	if err := json.Unmarshal(server.message(0), &got); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	gotPayload, ok := got.Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T, want map[string]any", got.Payload)
	}
	if got.Type != want.Type {
		t.Fatalf("Type = %q, want %q", got.Type, want.Type)
	}
	if gotPayload["jobId"] != "job-123" {
		t.Fatalf("payload.jobId = %#v, want %q", gotPayload["jobId"], "job-123")
	}
	if gotPayload["runtimeKind"] != "wails_desktop" {
		t.Fatalf("payload.runtimeKind = %#v, want %q", gotPayload["runtimeKind"], "wails_desktop")
	}
}

func TestOutboundManager_ScopesCacheByTenant(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()
	endpointID := uuid.New()
	serverA := newOutboundWSTestServer(t, "token-a")
	serverB := newOutboundWSTestServer(t, "token-b")

	endpointStore := &stubWorkerEndpointStore{
		endpointsByTenant: map[uuid.UUID]*store.WorkerEndpointData{
			tenantA: {
				BaseModel:   store.BaseModel{ID: endpointID},
				TenantID:    tenantA,
				Name:        "desktop-a",
				RuntimeKind: "wails_desktop",
				EndpointURL: httpToWebsocketURL(serverA.server.URL),
				AuthToken:   "token-a",
			},
			tenantB: {
				BaseModel:   store.BaseModel{ID: endpointID},
				TenantID:    tenantB,
				Name:        "desktop-b",
				RuntimeKind: "wails_desktop",
				EndpointURL: httpToWebsocketURL(serverB.server.URL),
				AuthToken:   "token-b",
			},
		},
	}
	mgr := NewOutboundManager(endpointStore)

	if err := mgr.Dispatch(store.WithTenantID(context.Background(), tenantA), endpointID, OutboundEnvelope{Type: OutboundEnvelopeJobDispatch, Payload: map[string]any{"jobId": "job-a"}}); err != nil {
		t.Fatalf("tenant A Dispatch error: %v", err)
	}
	serverA.waitForMessages(t, 1)

	if err := mgr.Dispatch(store.WithTenantID(context.Background(), tenantB), endpointID, OutboundEnvelope{Type: OutboundEnvelopeJobDispatch, Payload: map[string]any{"jobId": "job-b"}}); err != nil {
		t.Fatalf("tenant B Dispatch error: %v", err)
	}
	serverB.waitForMessages(t, 1)

	if got := endpointStore.getCalls; got != 2 {
		t.Fatalf("endpoint store Get calls = %d, want 2", got)
	}
	if got := serverA.connectCount(); got != 1 {
		t.Fatalf("tenant A connect count = %d, want 1", got)
	}
	if got := serverB.connectCount(); got != 1 {
		t.Fatalf("tenant B connect count = %d, want 1", got)
	}
	if got := serverA.messageCount(); got != 1 {
		t.Fatalf("tenant A message count = %d, want 1", got)
	}
	if got := serverB.messageCount(); got != 1 {
		t.Fatalf("tenant B message count = %d, want 1", got)
	}
	if got := serverA.headerValues(); len(got) != 1 || got[0] != "token-a" {
		t.Fatalf("tenant A Authorization headers = %#v, want [token-a]", got)
	}
	if got := serverB.headerValues(); len(got) != 1 || got[0] != "token-b" {
		t.Fatalf("tenant B Authorization headers = %#v, want [token-b]", got)
	}
}

func TestOutboundManager_DoesNotRetryWriteFailures(t *testing.T) {
	tenantID := uuid.New()
	endpointID := uuid.New()
	server := newOutboundWSTestServer(t, "token-123")

	endpointStore := &stubWorkerEndpointStore{
		endpoint: &store.WorkerEndpointData{
			BaseModel:   store.BaseModel{ID: endpointID},
			TenantID:    tenantID,
			Name:        "desktop-primary",
			RuntimeKind: "wails_desktop",
			EndpointURL: httpToWebsocketURL(server.server.URL),
			AuthToken:   "token-123",
		},
	}
	mgr := NewOutboundManager(endpointStore)
	ctx := store.WithTenantID(context.Background(), tenantID)

	if err := mgr.Dispatch(ctx, endpointID, OutboundEnvelope{Type: OutboundEnvelopeJobDispatch, Payload: map[string]any{"jobId": "job-1"}}); err != nil {
		t.Fatalf("first Dispatch error: %v", err)
	}
	server.waitForMessages(t, 1)

	server.closeAllConnections(t)

	err := mgr.Dispatch(ctx, endpointID, OutboundEnvelope{Type: OutboundEnvelopeJobDispatch, Payload: map[string]any{"jobId": "job-2"}})
	if err == nil {
		t.Fatal("Dispatch error = nil, want write failure")
	}

	time.Sleep(100 * time.Millisecond)
	if got := server.connectCount(); got != 1 {
		t.Fatalf("connect count = %d, want 1", got)
	}
	if got := server.messageCount(); got != 1 {
		t.Fatalf("message count = %d, want 1", got)
	}
}

func TestOutboundManager_HonorsContextOnSend(t *testing.T) {
	tenantID := uuid.New()
	endpointID := uuid.New()
	server := newOutboundWSTestServer(t, "token-123")

	endpointStore := &stubWorkerEndpointStore{
		endpoint: &store.WorkerEndpointData{
			BaseModel:   store.BaseModel{ID: endpointID},
			TenantID:    tenantID,
			Name:        "desktop-primary",
			RuntimeKind: "wails_desktop",
			EndpointURL: httpToWebsocketURL(server.server.URL),
			AuthToken:   "token-123",
		},
	}
	mgr := NewOutboundManager(endpointStore)
	ctx := store.WithTenantID(context.Background(), tenantID)

	if err := mgr.Dispatch(ctx, endpointID, OutboundEnvelope{Type: OutboundEnvelopeJobDispatch, Payload: map[string]any{"jobId": "job-1"}}); err != nil {
		t.Fatalf("first Dispatch error: %v", err)
	}
	server.waitForMessages(t, 1)

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	err := mgr.Dispatch(canceledCtx, endpointID, OutboundEnvelope{Type: OutboundEnvelopeJobDispatch, Payload: map[string]any{"jobId": "job-2"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Dispatch error = %v, want %v", err, context.Canceled)
	}

	time.Sleep(100 * time.Millisecond)
	if got := server.messageCount(); got != 1 {
		t.Fatalf("message count = %d, want 1", got)
	}
}

func TestOutboundManager_ForwardsWorkerRepliesToHandler(t *testing.T) {
	tenantID := uuid.New()
	endpointID := uuid.New()
	server := newOutboundWSTestServer(t, "token-123")
	handler := &recordingReplyHandler{replyCh: make(chan outboundReplyRecord, 1)}

	endpointStore := &stubWorkerEndpointStore{
		endpoint: &store.WorkerEndpointData{
			BaseModel:   store.BaseModel{ID: endpointID},
			TenantID:    tenantID,
			Name:        "desktop-primary",
			RuntimeKind: "wails_desktop",
			EndpointURL: httpToWebsocketURL(server.server.URL),
			AuthToken:   "token-123",
		},
	}
	mgr := NewOutboundManager(endpointStore)
	mgr.SetReplyHandler(handler)
	ctx := store.WithTenantID(context.Background(), tenantID)

	if err := mgr.Dispatch(ctx, endpointID, OutboundEnvelope{Type: OutboundEnvelopeJobDispatch, Payload: map[string]any{"jobId": "job-1"}}); err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	server.waitForMessages(t, 1)
	server.sendReply(t, WorkerReplyEnvelope{Type: "job.status", JobID: uuid.NewString(), Status: "streaming", Payload: json.RawMessage(`{"message":"running"}`)})

	reply := handler.waitForReply(t)
	if reply.tenantID != tenantID {
		t.Fatalf("tenant id = %s, want %s", reply.tenantID, tenantID)
	}
	if reply.endpointID != endpointID {
		t.Fatalf("endpoint id = %s, want %s", reply.endpointID, endpointID)
	}
	if reply.message.Type != "job.status" {
		t.Fatalf("reply type = %q, want %q", reply.message.Type, "job.status")
	}
	if reply.message.Status != "streaming" {
		t.Fatalf("reply status = %q, want %q", reply.message.Status, "streaming")
	}
}

type stubWorkerEndpointStore struct {
	endpoint          *store.WorkerEndpointData
	endpointsByTenant map[uuid.UUID]*store.WorkerEndpointData
	getErr            error
	getCalls          int
	createErr         error
}

type outboundReplyRecord struct {
	tenantID   uuid.UUID
	endpointID uuid.UUID
	message    WorkerReplyEnvelope
}

type recordingReplyHandler struct {
	replyCh chan outboundReplyRecord
}

func (h *recordingReplyHandler) HandleOutboundWorkerMessage(ctx context.Context, endpointID uuid.UUID, message WorkerReplyEnvelope) error {
	h.replyCh <- outboundReplyRecord{tenantID: store.TenantIDFromContext(ctx), endpointID: endpointID, message: message}
	return nil
}

func (h *recordingReplyHandler) waitForReply(t *testing.T) outboundReplyRecord {
	t.Helper()
	select {
	case reply := <-h.replyCh:
		return reply
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reply")
		return outboundReplyRecord{}
	}
}

func (s *stubWorkerEndpointStore) Create(context.Context, *store.WorkerEndpointData) error {
	return s.createErr
}

func (s *stubWorkerEndpointStore) Get(ctx context.Context, id uuid.UUID) (*store.WorkerEndpointData, error) {
	s.getCalls++
	if s.getErr != nil {
		return nil, s.getErr
	}
	if endpoint := s.endpointsByTenant[store.TenantIDFromContext(ctx)]; endpoint != nil && endpoint.ID == id {
		copy := *endpoint
		return &copy, nil
	}
	if s.endpoint == nil || s.endpoint.ID != id {
		return nil, nil
	}
	copy := *s.endpoint
	return &copy, nil
}

func (s *stubWorkerEndpointStore) List(context.Context) ([]store.WorkerEndpointData, error) {
	return nil, nil
}

func (s *stubWorkerEndpointStore) Update(context.Context, uuid.UUID, map[string]any) error {
	return nil
}

func (s *stubWorkerEndpointStore) Delete(context.Context, uuid.UUID) error {
	return nil
}

type outboundWSTestServer struct {
	t             *testing.T
	server        *httptest.Server
	requiredToken string

	mu           sync.Mutex
	connects     int
	headers      []string
	messages     [][]byte
	connections  []*websocket.Conn
	messageCh    chan struct{}
	upgradeError error
}

func newOutboundWSTestServer(t *testing.T, requiredToken string) *outboundWSTestServer {
	t.Helper()

	ts := &outboundWSTestServer{
		t:             t,
		requiredToken: requiredToken,
		messageCh:     make(chan struct{}, 8),
	}
	upgrader := websocket.Upgrader{}
	ts.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token != ts.requiredToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			ts.mu.Lock()
			ts.upgradeError = err
			ts.mu.Unlock()
			return
		}

		ts.mu.Lock()
		ts.connects++
		ts.headers = append(ts.headers, token)
		ts.connections = append(ts.connections, conn)
		ts.mu.Unlock()

		go func() {
			defer conn.Close()
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					return
				}
				ts.mu.Lock()
				ts.messages = append(ts.messages, append([]byte(nil), msg...))
				ts.mu.Unlock()
				ts.messageCh <- struct{}{}
			}
		}()
	}))
	t.Cleanup(func() {
		ts.server.Close()
	})
	return ts
}

func (s *outboundWSTestServer) sendReply(t *testing.T, reply WorkerReplyEnvelope) {
	t.Helper()

	s.mu.Lock()
	if len(s.connections) == 0 {
		s.mu.Unlock()
		t.Fatal("no active websocket connections")
	}
	conn := s.connections[len(s.connections)-1]
	s.mu.Unlock()

	if err := conn.WriteJSON(reply); err != nil {
		t.Fatalf("write reply: %v", err)
	}
}

func (s *outboundWSTestServer) waitForMessages(t *testing.T, want int) {
	t.Helper()
	for i := 0; i < want; i++ {
		select {
		case <-s.messageCh:
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for message %d of %d", i+1, want)
		}
	}
}

func (s *outboundWSTestServer) connectCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connects
}

func (s *outboundWSTestServer) closeAllConnections(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	conns := append([]*websocket.Conn(nil), s.connections...)
	s.mu.Unlock()
	for _, conn := range conns {
		if conn == nil {
			continue
		}
		_ = conn.Close()
	}
}

func (s *outboundWSTestServer) headerValues() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.headers...)
}

func (s *outboundWSTestServer) message(index int) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.upgradeError != nil {
		s.t.Fatalf("upgrade error: %v", s.upgradeError)
	}
	if index >= len(s.messages) {
		s.t.Fatalf("message index %d out of range %d", index, len(s.messages))
	}
	return append([]byte(nil), s.messages[index]...)
}

func (s *outboundWSTestServer) messageCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

func httpToWebsocketURL(raw string) string {
	if strings.HasPrefix(raw, "https://") {
		return "wss://" + strings.TrimPrefix(raw, "https://")
	}
	if strings.HasPrefix(raw, "http://") {
		return "ws://" + strings.TrimPrefix(raw, "http://")
	}
	return raw
}

func cachedOutboundClient(t *testing.T, mgr *OutboundManager, tenantID, endpointID uuid.UUID) *outboundClient {
	t.Helper()

	for key, client := range mgr.clients {
		if key.tenantID == tenantID && key.endpointID == endpointID && client != nil {
			return client
		}
	}
	t.Fatalf("cached outbound client not found for tenant %s endpoint %s", tenantID, endpointID)
	return nil
}

var _ store.WorkerEndpointStore = (*stubWorkerEndpointStore)(nil)
