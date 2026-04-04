package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type httpWorkerEndpointStore struct {
	items        []store.WorkerEndpointData
	created      *store.WorkerEndpointData
	updatedID    uuid.UUID
	updated      map[string]any
	deletedID    uuid.UUID
	getByID      map[uuid.UUID]*store.WorkerEndpointData
	listCalled   bool
	createCalled bool
	updateCalled bool
	deleteCalled bool
}

func (s *httpWorkerEndpointStore) Create(_ context.Context, endpoint *store.WorkerEndpointData) error {
	s.createCalled = true
	copyValue := *endpoint
	s.created = &copyValue
	return nil
}

func (s *httpWorkerEndpointStore) Get(_ context.Context, id uuid.UUID) (*store.WorkerEndpointData, error) {
	if s.getByID == nil {
		return nil, nil
	}
	return s.getByID[id], nil
}

func (s *httpWorkerEndpointStore) List(context.Context) ([]store.WorkerEndpointData, error) {
	s.listCalled = true
	return s.items, nil
}

func (s *httpWorkerEndpointStore) Update(_ context.Context, id uuid.UUID, updates map[string]any) error {
	s.updateCalled = true
	s.updatedID = id
	s.updated = updates
	return nil
}

func (s *httpWorkerEndpointStore) Delete(_ context.Context, id uuid.UUID) error {
	s.deleteCalled = true
	s.deletedID = id
	return nil
}

func TestWorkerEndpointsHandler_CRUDRoutes(t *testing.T) {
	endpointID := uuid.New()
	storeStub := &httpWorkerEndpointStore{
		items: []store.WorkerEndpointData{{
			BaseModel:   store.BaseModel{ID: endpointID},
			TenantID:    store.MasterTenantID,
			Name:        "desktop-a",
			RuntimeKind: "wails_desktop",
			EndpointURL: "http://127.0.0.1:18790",
			AuthToken:   "token-a",
		}},
		getByID: map[uuid.UUID]*store.WorkerEndpointData{
			endpointID: {
				BaseModel:   store.BaseModel{ID: endpointID},
				TenantID:    store.MasterTenantID,
				Name:        "desktop-a",
				RuntimeKind: "wails_desktop",
				EndpointURL: "http://127.0.0.1:18790",
				AuthToken:   "token-a",
			},
		},
	}

	handler := NewWorkerEndpointsHandler(storeStub, nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	t.Run("list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/worker-endpoints", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
		}
		if !storeStub.listCalled {
			t.Fatal("List was not called")
		}
		var resp struct {
			Items []store.WorkerEndpointData `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal list response: %v", err)
		}
		if len(resp.Items) != 1 {
			t.Fatalf("items len = %d, want 1", len(resp.Items))
		}
		if resp.Items[0].AuthToken != "" {
			t.Fatalf("list auth_token = %q, want empty", resp.Items[0].AuthToken)
		}
	})

	t.Run("create", func(t *testing.T) {
		body := bytes.NewBufferString(`{"name":"desktop-new","runtime_kind":"wails_desktop","endpoint_url":"http://127.0.0.1:18791","auth_token":"token-new"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/worker-endpoints", body)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusCreated, w.Body.String())
		}
		if !storeStub.createCalled || storeStub.created == nil {
			t.Fatal("Create was not called")
		}
		if storeStub.created.Name != "desktop-new" {
			t.Fatalf("created Name = %q, want %q", storeStub.created.Name, "desktop-new")
		}
		var resp store.WorkerEndpointData
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal create response: %v", err)
		}
		if resp.AuthToken != "" {
			t.Fatalf("create auth_token = %q, want empty", resp.AuthToken)
		}
	})

	t.Run("update", func(t *testing.T) {
		body := bytes.NewBufferString(`{"name":"desktop-updated","endpoint_url":"http://127.0.0.1:18792","auth_token":"token-updated"}`)
		req := httptest.NewRequest(http.MethodPut, "/v1/worker-endpoints/"+endpointID.String(), body)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
		}
		if !storeStub.updateCalled {
			t.Fatal("Update was not called")
		}
		if storeStub.updatedID != endpointID {
			t.Fatalf("updated ID = %v, want %v", storeStub.updatedID, endpointID)
		}
		if got, _ := storeStub.updated["name"].(string); got != "desktop-updated" {
			t.Fatalf("updated name = %#v, want %q", storeStub.updated["name"], "desktop-updated")
		}
	})

	t.Run("delete", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v1/worker-endpoints/"+endpointID.String(), nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
		}
		if !storeStub.deleteCalled {
			t.Fatal("Delete was not called")
		}
		if storeStub.deletedID != endpointID {
			t.Fatalf("deleted ID = %v, want %v", storeStub.deletedID, endpointID)
		}
	})
}
