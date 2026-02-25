package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bootstrap"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// AgentsHandler handles agent CRUD and sharing endpoints (managed mode only).
type AgentsHandler struct {
	agents   store.AgentStore
	token    string
	msgBus   *bus.MessageBus  // for cache invalidation events (nil = no events)
	summoner *AgentSummoner   // LLM-based agent setup (nil = disabled)
}

// NewAgentsHandler creates a handler for agent management endpoints.
func NewAgentsHandler(agents store.AgentStore, token string, msgBus *bus.MessageBus, summoner *AgentSummoner) *AgentsHandler {
	return &AgentsHandler{agents: agents, token: token, msgBus: msgBus, summoner: summoner}
}

// emitCacheInvalidate broadcasts a cache invalidation event if msgBus is set.
func (h *AgentsHandler) emitCacheInvalidate(kind, key string) {
	if h.msgBus == nil {
		return
	}
	h.msgBus.Broadcast(bus.Event{
		Name:    protocol.EventCacheInvalidate,
		Payload: bus.CacheInvalidatePayload{Kind: kind, Key: key},
	})
}

// RegisterRoutes registers all agent management routes on the given mux.
func (h *AgentsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/agents", h.authMiddleware(h.handleList))
	mux.HandleFunc("POST /v1/agents", h.authMiddleware(h.handleCreate))
	mux.HandleFunc("GET /v1/agents/{id}", h.authMiddleware(h.handleGet))
	mux.HandleFunc("PUT /v1/agents/{id}", h.authMiddleware(h.handleUpdate))
	mux.HandleFunc("DELETE /v1/agents/{id}", h.authMiddleware(h.handleDelete))
	mux.HandleFunc("GET /v1/agents/{id}/shares", h.authMiddleware(h.handleListShares))
	mux.HandleFunc("POST /v1/agents/{id}/shares", h.authMiddleware(h.handleShare))
	mux.HandleFunc("DELETE /v1/agents/{id}/shares/{userID}", h.authMiddleware(h.handleRevokeShare))
	mux.HandleFunc("POST /v1/agents/{id}/regenerate", h.authMiddleware(h.handleRegenerate))
	mux.HandleFunc("POST /v1/agents/{id}/resummon", h.authMiddleware(h.handleResummon))
}

func (h *AgentsHandler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.token != "" {
			if extractBearerToken(r) != h.token {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
		}
		// Inject user_id into context
		userID := extractUserID(r)
		if userID != "" {
			ctx := store.WithUserID(r.Context(), userID)
			r = r.WithContext(ctx)
		}
		next(w, r)
	}
}

func (h *AgentsHandler) handleList(w http.ResponseWriter, r *http.Request) {
	userID := store.UserIDFromContext(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-GoClaw-User-Id header required"})
		return
	}

	agents, err := h.agents.ListAccessible(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"agents": agents})
}

func (h *AgentsHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	userID := store.UserIDFromContext(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-GoClaw-User-Id header required"})
		return
	}

	var req store.AgentData
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	if !isValidSlug(req.AgentKey) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_key must be a valid slug (lowercase letters, numbers, hyphens only)"})
		return
	}

	req.OwnerID = userID
	if req.AgentType == "" {
		req.AgentType = store.AgentTypeOpen
	}
	if req.ContextWindow <= 0 {
		req.ContextWindow = 200000
	}
	if req.MaxToolIterations <= 0 {
		req.MaxToolIterations = 20
	}
	if req.Workspace == "" {
		if req.IsDefault {
			req.Workspace = "~/.goclaw/workspace"
		} else {
			req.Workspace = fmt.Sprintf("~/.goclaw/%s-workspace", req.AgentKey)
		}
	}
	req.RestrictToWorkspace = true

	// Default: enable compaction and memory for new agents
	if len(req.CompactionConfig) == 0 {
		req.CompactionConfig = json.RawMessage(`{}`)
	}
	if len(req.MemoryConfig) == 0 {
		req.MemoryConfig = json.RawMessage(`{"enabled":true}`)
	}

	// Check if predefined agent has a description for LLM summoning
	description := extractDescription(req.OtherConfig)
	if req.AgentType == store.AgentTypePredefined && description != "" && h.summoner != nil {
		req.Status = store.AgentStatusSummoning
	} else if req.Status == "" {
		req.Status = store.AgentStatusActive
	}

	if err := h.agents.Create(r.Context(), &req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Seed context files into agent_context_files (skipped for open agents).
	// For summoning agents, templates serve as fallback if LLM fails.
	if _, err := bootstrap.SeedToStore(r.Context(), h.agents, req.ID, req.AgentType); err != nil {
		slog.Warn("failed to seed context files for new agent", "agent", req.AgentKey, "error", err)
	}

	// Start LLM summoning in background if applicable
	if req.Status == store.AgentStatusSummoning {
		go h.summoner.SummonAgent(req.ID, req.Provider, req.Model, description)
	}

	writeJSON(w, http.StatusCreated, req)
}

func (h *AgentsHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	userID := store.UserIDFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		// Try by agent_key
		ag, err2 := h.agents.GetByKey(r.Context(), r.PathValue("id"))
		if err2 != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		if userID != "" {
			if ok, _, _ := h.agents.CanAccess(r.Context(), ag.ID, userID); !ok {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "no access to this agent"})
				return
			}
		}
		writeJSON(w, http.StatusOK, ag)
		return
	}

	ag, err := h.agents.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}

	if userID != "" {
		if ok, _, _ := h.agents.CanAccess(r.Context(), id, userID); !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "no access to this agent"})
			return
		}
	}

	writeJSON(w, http.StatusOK, ag)
}

func (h *AgentsHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	userID := store.UserIDFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}

	// Only owner can update
	ag, err := h.agents.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if userID != "" && ag.OwnerID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owner can update agent"})
		return
	}

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	// Prevent changing owner_id
	delete(updates, "owner_id")
	delete(updates, "id")

	if err := h.agents.Update(r.Context(), id, updates); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Invalidate caches: agent Loop + bootstrap files
	h.emitCacheInvalidate(bus.CacheKindAgent, ag.AgentKey)
	h.emitCacheInvalidate(bus.CacheKindBootstrap, id.String())

	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *AgentsHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	userID := store.UserIDFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}

	// Only owner can delete
	ag, err := h.agents.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if userID != "" && ag.OwnerID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owner can delete agent"})
		return
	}

	if err := h.agents.Delete(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Invalidate caches: agent Loop + bootstrap files
	h.emitCacheInvalidate(bus.CacheKindAgent, ag.AgentKey)
	h.emitCacheInvalidate(bus.CacheKindBootstrap, id.String())

	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *AgentsHandler) handleListShares(w http.ResponseWriter, r *http.Request) {
	userID := store.UserIDFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}

	// Only owner can list shares
	ag, err := h.agents.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if userID != "" && ag.OwnerID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owner can view shares"})
		return
	}

	shares, err := h.agents.ListShares(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"shares": shares})
}

func (h *AgentsHandler) handleShare(w http.ResponseWriter, r *http.Request) {
	userID := store.UserIDFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}

	// Only owner can share
	ag, err := h.agents.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if userID != "" && ag.OwnerID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owner can share agent"})
		return
	}

	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.UserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}
	if err := store.ValidateUserID(req.UserID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Role == "" {
		req.Role = "user"
	}

	if err := h.agents.ShareAgent(r.Context(), id, req.UserID, req.Role, userID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"ok": "true"})
}

func (h *AgentsHandler) handleRevokeShare(w http.ResponseWriter, r *http.Request) {
	userID := store.UserIDFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}

	// Only owner can revoke shares
	ag, err := h.agents.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if userID != "" && ag.OwnerID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owner can revoke shares"})
		return
	}

	targetUserID := r.PathValue("userID")
	if err := store.ValidateUserID(targetUserID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.agents.RevokeShare(r.Context(), id, targetUserID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *AgentsHandler) handleRegenerate(w http.ResponseWriter, r *http.Request) {
	userID := store.UserIDFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}

	// Only owner can regenerate
	ag, err := h.agents.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if userID != "" && ag.OwnerID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owner can regenerate agent"})
		return
	}
	if ag.Status == store.AgentStatusSummoning {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "agent is already being summoned"})
		return
	}
	if h.summoner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summoning not available"})
		return
	}

	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		return
	}

	// Set status to summoning
	if err := h.agents.Update(r.Context(), id, map[string]any{"status": store.AgentStatusSummoning}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	go h.summoner.RegenerateAgent(id, ag.Provider, ag.Model, req.Prompt)

	writeJSON(w, http.StatusAccepted, map[string]string{"ok": "true", "status": store.AgentStatusSummoning})
}

// handleResummon re-runs SummonAgent from scratch using the original description.
// Used when initial summoning failed (e.g. wrong model) and user wants to retry.
func (h *AgentsHandler) handleResummon(w http.ResponseWriter, r *http.Request) {
	userID := store.UserIDFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}

	ag, err := h.agents.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if userID != "" && ag.OwnerID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owner can resummon agent"})
		return
	}
	if ag.Status == store.AgentStatusSummoning {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "agent is already being summoned"})
		return
	}
	if h.summoner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summoning not available"})
		return
	}

	description := extractDescription(ag.OtherConfig)
	if description == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent has no description to resummon from"})
		return
	}

	if err := h.agents.Update(r.Context(), id, map[string]any{"status": store.AgentStatusSummoning}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	go h.summoner.SummonAgent(id, ag.Provider, ag.Model, description)

	writeJSON(w, http.StatusAccepted, map[string]string{"ok": "true", "status": store.AgentStatusSummoning})
}

// extractDescription pulls the description string from other_config JSONB.
func extractDescription(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var cfg map[string]interface{}
	if json.Unmarshal(raw, &cfg) != nil {
		return ""
	}
	desc, _ := cfg["description"].(string)
	return desc
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

