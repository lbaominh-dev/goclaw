package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

type WorkerEndpointsHandler struct {
	store  store.WorkerEndpointStore
	msgBus *bus.MessageBus
}

func NewWorkerEndpointsHandler(s store.WorkerEndpointStore, msgBus *bus.MessageBus) *WorkerEndpointsHandler {
	return &WorkerEndpointsHandler{store: s, msgBus: msgBus}
}

func (h *WorkerEndpointsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/worker-endpoints", h.auth(h.handleList))
	mux.HandleFunc("POST /v1/worker-endpoints", h.auth(h.handleCreate))
	mux.HandleFunc("PUT /v1/worker-endpoints/{id}", h.auth(h.handleUpdate))
	mux.HandleFunc("DELETE /v1/worker-endpoints/{id}", h.auth(h.handleDelete))
}

func (h *WorkerEndpointsHandler) auth(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth(permissions.RoleAdmin, next)
}

func (h *WorkerEndpointsHandler) handleList(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	items, err := h.store.List(r.Context())
	if err != nil {
		slog.Error("worker_endpoints.list", "error", err)
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "worker endpoints"))
		return
	}
	for i := range items {
		items[i].AuthToken = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *WorkerEndpointsHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	var req store.WorkerEndpointData
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON))
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.RuntimeKind) == "" || strings.TrimSpace(req.EndpointURL) == "" || strings.TrimSpace(req.AuthToken) == "" {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "name, runtime_kind, endpoint_url, auth_token"))
		return
	}
	if err := h.store.Create(r.Context(), &req); err != nil {
		slog.Error("worker_endpoints.create", "error", err)
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToCreate, "worker endpoint", "internal error"))
		return
	}
	req.AuthToken = ""
	writeJSON(w, http.StatusCreated, req)
}

func (h *WorkerEndpointsHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "worker endpoint"))
		return
	}

	var updates map[string]any
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON))
		return
	}
	updates = filterAllowedKeys(updates, workerEndpointAllowedFields)
	if err := h.store.Update(r.Context(), id, updates); err != nil {
		slog.Error("worker_endpoints.update", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "worker endpoint", "internal error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *WorkerEndpointsHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "worker endpoint"))
		return
	}
	if err := h.store.Delete(r.Context(), id); err != nil {
		slog.Error("worker_endpoints.delete", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToDelete, "worker endpoint", "internal error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}
