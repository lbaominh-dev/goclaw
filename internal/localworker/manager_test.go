package localworker

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

type stubConnection struct {
	last  Envelope
	calls int
}

func (c *stubConnection) Dispatch(_ context.Context, env Envelope) error {
	c.calls++
	c.last = env
	return nil
}

type blockingConnection struct {
	entered chan struct{}
	release chan struct{}
	calls   int
}

func (c *blockingConnection) Dispatch(_ context.Context, _ Envelope) error {
	c.calls++
	close(c.entered)
	<-c.release
	return nil
}

func TestManager_RegisterDispatchDisconnect(t *testing.T) {
	tenantID := uuid.New()
	workerID := "worker-1"
	conn := &stubConnection{}

	mgr := NewManager()
	if mgr.IsOnline(tenantID, workerID) {
		t.Fatal("worker should start offline")
	}
	if registered, err := mgr.Register(tenantID, workerID, nil); !errors.Is(err, ErrWorkerConnectionRequired) {
		t.Fatalf("Register error = %v, want %v", err, ErrWorkerConnectionRequired)
	} else if registered != nil {
		t.Fatal("Register should return nil registration for nil connection")
	}
	if mgr.IsOnline(tenantID, workerID) {
		t.Fatal("nil connection should not mark worker online")
	}

	registered, err := mgr.Register(tenantID, workerID, conn)
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if registered == nil {
		t.Fatal("Register returned nil")
	}
	if registered.TenantID != tenantID {
		t.Fatalf("TenantID = %s, want %s", registered.TenantID, tenantID)
	}
	if registered.WorkerID != workerID {
		t.Fatalf("WorkerID = %q, want %q", registered.WorkerID, workerID)
	}
	if registered.Connection != conn {
		t.Fatal("Connection mismatch")
	}
	registered.Connection = nil

	resolved, ok := mgr.Get(tenantID, workerID)
	if !ok {
		t.Fatal("Get should resolve registered worker")
	}
	if resolved.Connection != conn {
		t.Fatal("Get should expose registered connection state")
	}
	resolved.Connection = nil
	if !mgr.IsOnline(tenantID, workerID) {
		t.Fatal("worker should be online after Register")
	}

	env := Envelope{
		Type:     "job.dispatch",
		TenantID: uuid.New(),
		WorkerID: "different-worker",
		Payload: map[string]any{
			"jobId": "job-123",
		},
	}
	if err := mgr.Dispatch(context.Background(), tenantID, workerID, env); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	env.TenantID = tenantID
	env.WorkerID = workerID
	if conn.calls != 1 {
		t.Fatalf("Dispatch call count = %d, want 1", conn.calls)
	}
	if !reflect.DeepEqual(conn.last, env) {
		t.Fatalf("Dispatch envelope = %#v, want %#v", conn.last, env)
	}

	blockingWorkerID := "worker-blocking"
	blockingConn := &blockingConnection{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	if _, err := mgr.Register(tenantID, blockingWorkerID, blockingConn); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	dispatchErrCh := make(chan error, 1)
	go func() {
		dispatchErrCh <- mgr.Dispatch(context.Background(), tenantID, blockingWorkerID, Envelope{Type: "job.dispatch"})
	}()
	select {
	case <-blockingConn.entered:
	case <-time.After(1 * time.Second):
		t.Fatal("dispatch did not reach connection")
	}
	disconnectDone := make(chan bool, 1)
	go func() {
		disconnectDone <- mgr.Disconnect(tenantID, blockingWorkerID)
	}()
	select {
	case <-disconnectDone:
		t.Fatal("Disconnect should wait for in-progress dispatch to finish")
	case <-time.After(20 * time.Millisecond):
	}
	close(blockingConn.release)
	if err := <-dispatchErrCh; err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	select {
	case disconnected := <-disconnectDone:
		if !disconnected {
			t.Fatal("Disconnect should report true for existing worker")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Disconnect did not complete after dispatch finished")
	}
	if err := mgr.Dispatch(context.Background(), tenantID, blockingWorkerID, Envelope{Type: "job.dispatch"}); !errors.Is(err, ErrWorkerNotConnected) {
		t.Fatalf("Dispatch error = %v, want %v", err, ErrWorkerNotConnected)
	}

	otherTenant := uuid.New()
	if _, err := mgr.Register(otherTenant, workerID, &stubConnection{}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	mgr.Disconnect(tenantID, workerID)
	if mgr.IsOnline(tenantID, workerID) {
		t.Fatal("worker should be offline after Disconnect")
	}
	if _, ok := mgr.Get(tenantID, workerID); ok {
		t.Fatal("Get should not resolve disconnected worker")
	}
	if !mgr.IsOnline(otherTenant, workerID) {
		t.Fatal("disconnect should not affect same worker ID in another tenant")
	}

	if err := mgr.Dispatch(context.Background(), tenantID, workerID, env); !errors.Is(err, ErrWorkerNotConnected) {
		t.Fatalf("Dispatch error = %v, want %v", err, ErrWorkerNotConnected)
	}
	if err := mgr.Dispatch(context.Background(), uuid.New(), "missing-worker", env); !errors.Is(err, ErrWorkerNotConnected) {
		t.Fatalf("Dispatch error = %v, want %v", err, ErrWorkerNotConnected)
	}
	if mgr.Disconnect(uuid.New(), "missing-worker") {
		t.Fatal("Disconnect should report false for unknown worker")
	}
	if !mgr.Disconnect(otherTenant, workerID) {
		t.Fatal("Disconnect should report true for existing worker")
	}
}
