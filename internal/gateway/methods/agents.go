package methods

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/bootstrap"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// AgentsMethods handles agents.list, agents.create, agents.update, agents.delete,
// agents.files.list/get/set, agent.identity.get.
type AgentsMethods struct {
	agents     *agent.Router
	cfg        *config.Config
	cfgPath    string
	workspace  string
	agentStore store.AgentStore // nil in standalone mode
	isManaged  bool
}

func NewAgentsMethods(agents *agent.Router, cfg *config.Config, cfgPath, workspace string, agentStore store.AgentStore, isManaged bool) *AgentsMethods {
	return &AgentsMethods{agents: agents, cfg: cfg, cfgPath: cfgPath, workspace: workspace, agentStore: agentStore, isManaged: isManaged}
}

func (m *AgentsMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodAgent, m.handleAgent)
	router.Register(protocol.MethodAgentWait, m.handleAgentWait)
	router.Register(protocol.MethodAgentsList, m.handleList)
	router.Register(protocol.MethodAgentsCreate, m.handleCreate)
	router.Register(protocol.MethodAgentsUpdate, m.handleUpdate)
	router.Register(protocol.MethodAgentsDelete, m.handleDelete)
	router.Register(protocol.MethodAgentsFileList, m.handleFilesList)
	router.Register(protocol.MethodAgentsFileGet, m.handleFilesGet)
	router.Register(protocol.MethodAgentsFileSet, m.handleFilesSet)
	router.Register(protocol.MethodAgentIdentityGet, m.handleIdentityGet)
}

type agentParams struct {
	AgentID string `json:"agentId"`
}

func (m *AgentsMethods) handleAgent(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params agentParams
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}
	if params.AgentID == "" {
		params.AgentID = "default"
	}

	loop, err := m.agents.Get(params.AgentID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, err.Error()))
		return
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"id":        loop.ID(),
		"isRunning": loop.IsRunning(),
	}))
}

func (m *AgentsMethods) handleAgentWait(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params agentParams
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}
	if params.AgentID == "" {
		params.AgentID = "default"
	}

	loop, err := m.agents.Get(params.AgentID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, err.Error()))
		return
	}

	// Return current status (blocking wait is a future enhancement).
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"id":     loop.ID(),
		"status": "idle",
	}))
}

func (m *AgentsMethods) handleList(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	infos := m.agents.ListInfo()
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"agents": infos,
	}))
}

// --- agents.create ---
// Matching TS src/gateway/server-methods/agents.ts:216-287

func (m *AgentsMethods) handleCreate(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params struct {
		Name      string `json:"name"`
		Workspace string `json:"workspace"`
		Emoji     string `json:"emoji"`
		Avatar    string `json:"avatar"`
		AgentType string `json:"agent_type"` // "open" (default) or "predefined"
		// Per-agent config overrides (managed mode only)
		ToolsConfig      json.RawMessage `json:"tools_config,omitempty"`
		SubagentsConfig  json.RawMessage `json:"subagents_config,omitempty"`
		SandboxConfig    json.RawMessage `json:"sandbox_config,omitempty"`
		MemoryConfig     json.RawMessage `json:"memory_config,omitempty"`
		CompactionConfig json.RawMessage `json:"compaction_config,omitempty"`
		ContextPruning   json.RawMessage `json:"context_pruning,omitempty"`
		OtherConfig      json.RawMessage `json:"other_config,omitempty"`
	}
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}

	if params.Name == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "name is required"))
		return
	}

	agentType := params.AgentType
	if agentType == "" {
		agentType = store.AgentTypeOpen
	}

	agentID := config.NormalizeAgentID(params.Name)
	if agentID == "default" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "cannot create agent with reserved id 'default'"))
		return
	}

	// Resolve workspace
	ws := params.Workspace
	if ws == "" {
		ws = filepath.Join(m.workspace, "agents", agentID)
	} else {
		ws = config.ExpandHome(ws)
	}

	if m.isManaged && m.agentStore != nil {
		// --- Managed mode: create agent in DB ---
		ctx := context.Background()

		// Check if agent already exists in DB
		if existing, _ := m.agentStore.GetByKey(ctx, agentID); existing != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "agent already exists: "+agentID))
			return
		}

		agentData := &store.AgentData{
			AgentKey:         agentID,
			DisplayName:      params.Name,
			OwnerID:          "system",
			AgentType:        agentType,
			Provider:         m.cfg.Agents.Defaults.Provider,
			Model:            m.cfg.Agents.Defaults.Model,
			Workspace:        ws,
			Status:           store.AgentStatusActive,
			ToolsConfig:      params.ToolsConfig,
			SubagentsConfig:  params.SubagentsConfig,
			SandboxConfig:    params.SandboxConfig,
			MemoryConfig:     params.MemoryConfig,
			CompactionConfig: params.CompactionConfig,
			ContextPruning:   params.ContextPruning,
			OtherConfig:      params.OtherConfig,
		}
		if err := m.agentStore.Create(ctx, agentData); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, fmt.Sprintf("failed to create agent: %v", err)))
			return
		}

		// Seed context files to DB (skipped for open agents)
		if _, err := bootstrap.SeedToStore(ctx, m.agentStore, agentData.ID, agentData.AgentType); err != nil {
			slog.Warn("failed to seed bootstrap for agent", "agent", agentID, "error", err)
		}

		// Set identity in DB bootstrap
		if params.Name != "" || params.Emoji != "" || params.Avatar != "" {
			content := buildIdentityContent(params.Name, params.Emoji, params.Avatar)
			if err := m.agentStore.SetAgentContextFile(ctx, agentData.ID, "IDENTITY.md", content); err != nil {
				slog.Warn("failed to set IDENTITY.md", "agent", agentID, "error", err)
			}
		}

		// Invalidate router cache so resolver re-loads from DB
		m.agents.InvalidateAgent(agentID)
	} else {
		// --- Standalone mode: config.json + filesystem ---
		if _, ok := m.cfg.Agents.List[agentID]; ok {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "agent already exists: "+agentID))
			return
		}

		spec := config.AgentSpec{
			DisplayName: params.Name,
			Workspace:   ws,
		}
		if params.Emoji != "" || params.Avatar != "" {
			spec.Identity = &config.IdentityConfig{
				Emoji: params.Emoji,
			}
		}

		if m.cfg.Agents.List == nil {
			m.cfg.Agents.List = make(map[string]config.AgentSpec)
		}
		m.cfg.Agents.List[agentID] = spec

		if err := config.Save(m.cfgPath, m.cfg); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to save config: "+err.Error()))
			return
		}

		// Append identity metadata to IDENTITY.md
		if params.Name != "" || params.Emoji != "" || params.Avatar != "" {
			identityPath := filepath.Join(ws, "IDENTITY.md")
			appendIdentityFields(identityPath, params.Name, params.Emoji, params.Avatar)
		}
	}

	// Both modes: create workspace dir + seed filesystem backup
	os.MkdirAll(ws, 0755)
	bootstrap.EnsureWorkspaceFiles(ws)

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"ok":        true,
		"agentId":   agentID,
		"name":      params.Name,
		"workspace": ws,
	}))
}

// --- agents.update ---
// Matching TS src/gateway/server-methods/agents.ts:288-346

func (m *AgentsMethods) handleUpdate(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params struct {
		AgentID   string `json:"agentId"`
		Name      string `json:"name"`
		Workspace string `json:"workspace"`
		Model     string `json:"model"`
		Avatar    string `json:"avatar"`
		// Per-agent config overrides (managed mode only)
		ToolsConfig      json.RawMessage `json:"tools_config,omitempty"`
		SubagentsConfig  json.RawMessage `json:"subagents_config,omitempty"`
		SandboxConfig    json.RawMessage `json:"sandbox_config,omitempty"`
		MemoryConfig     json.RawMessage `json:"memory_config,omitempty"`
		CompactionConfig json.RawMessage `json:"compaction_config,omitempty"`
		ContextPruning   json.RawMessage `json:"context_pruning,omitempty"`
		OtherConfig      json.RawMessage `json:"other_config,omitempty"`
	}
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}

	if params.AgentID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "agentId is required"))
		return
	}

	if m.isManaged && m.agentStore != nil {
		// --- Managed mode: update agent in DB ---
		ctx := context.Background()
		ag, err := m.agentStore.GetByKey(ctx, params.AgentID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, "agent not found: "+params.AgentID))
			return
		}

		updates := map[string]any{}
		if params.Name != "" {
			updates["display_name"] = params.Name
		}
		if params.Workspace != "" {
			ws := config.ExpandHome(params.Workspace)
			updates["workspace"] = ws
			os.MkdirAll(ws, 0755)
		}
		if params.Model != "" {
			updates["model"] = params.Model
		}
		// Per-agent JSONB config overrides
		if len(params.ToolsConfig) > 0 {
			updates["tools_config"] = []byte(params.ToolsConfig)
		}
		if len(params.SubagentsConfig) > 0 {
			updates["subagents_config"] = []byte(params.SubagentsConfig)
		}
		if len(params.SandboxConfig) > 0 {
			updates["sandbox_config"] = []byte(params.SandboxConfig)
		}
		if len(params.MemoryConfig) > 0 {
			updates["memory_config"] = []byte(params.MemoryConfig)
		}
		if len(params.CompactionConfig) > 0 {
			updates["compaction_config"] = []byte(params.CompactionConfig)
		}
		if len(params.ContextPruning) > 0 {
			updates["context_pruning"] = []byte(params.ContextPruning)
		}
		if len(params.OtherConfig) > 0 {
			updates["other_config"] = []byte(params.OtherConfig)
		}

		if len(updates) > 0 {
			if err := m.agentStore.Update(ctx, ag.ID, updates); err != nil {
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, fmt.Sprintf("failed to update agent: %v", err)))
				return
			}
		}

		// Update identity in DB bootstrap
		if params.Avatar != "" || params.Name != "" {
			content := buildIdentityContent(params.Name, "", params.Avatar)
			if err := m.agentStore.SetAgentContextFile(ctx, ag.ID, "IDENTITY.md", content); err != nil {
				slog.Warn("failed to update IDENTITY.md", "agent", params.AgentID, "error", err)
			}
		}

		m.agents.InvalidateAgent(params.AgentID)
	} else {
		// --- Standalone mode: config.json ---
		spec, ok := m.cfg.Agents.List[params.AgentID]
		if !ok {
			if params.AgentID != "default" {
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, "agent not found: "+params.AgentID))
				return
			}
		}

		if params.Name != "" {
			spec.DisplayName = params.Name
		}
		if params.Workspace != "" {
			spec.Workspace = config.ExpandHome(params.Workspace)
			os.MkdirAll(spec.Workspace, 0755)
		}
		if params.Model != "" {
			spec.Model = params.Model
		}

		if params.AgentID == "default" {
			if params.Model != "" {
				m.cfg.Agents.Defaults.Model = params.Model
			}
			if params.Workspace != "" {
				m.cfg.Agents.Defaults.Workspace = params.Workspace
			}
		} else {
			m.cfg.Agents.List[params.AgentID] = spec
		}

		if params.Avatar != "" {
			ws := spec.Workspace
			if ws == "" {
				ws = config.ExpandHome(m.cfg.Agents.Defaults.Workspace)
			}
			identityPath := filepath.Join(ws, "IDENTITY.md")
			appendIdentityFields(identityPath, "", "", params.Avatar)
		}

		if err := config.Save(m.cfgPath, m.cfg); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to save config: "+err.Error()))
			return
		}
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"ok":      true,
		"agentId": params.AgentID,
	}))
}

// --- agents.delete ---
// Matching TS src/gateway/server-methods/agents.ts:347-398

func (m *AgentsMethods) handleDelete(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params struct {
		AgentID     string `json:"agentId"`
		DeleteFiles bool   `json:"deleteFiles"`
	}
	params.DeleteFiles = true // default
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}

	if params.AgentID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "agentId is required"))
		return
	}
	if params.AgentID == "default" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "cannot delete the default agent"))
		return
	}

	var removedBindings int

	if m.isManaged && m.agentStore != nil {
		// --- Managed mode: delete from DB ---
		ctx := context.Background()
		ag, err := m.agentStore.GetByKey(ctx, params.AgentID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, "agent not found: "+params.AgentID))
			return
		}

		if err := m.agentStore.Delete(ctx, ag.ID); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, fmt.Sprintf("failed to delete agent: %v", err)))
			return
		}

		m.agents.InvalidateAgent(params.AgentID)
		m.agents.Remove(params.AgentID)

		// Best-effort delete workspace
		if params.DeleteFiles && ag.Workspace != "" {
			os.RemoveAll(ag.Workspace)
		}
	} else {
		// --- Standalone mode: config.json ---
		spec, ok := m.cfg.Agents.List[params.AgentID]
		if !ok {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, "agent not found: "+params.AgentID))
			return
		}

		delete(m.cfg.Agents.List, params.AgentID)

		var kept []config.AgentBinding
		for _, b := range m.cfg.Bindings {
			if b.AgentID == params.AgentID {
				removedBindings++
			} else {
				kept = append(kept, b)
			}
		}
		m.cfg.Bindings = kept

		m.agents.Remove(params.AgentID)

		if err := config.Save(m.cfgPath, m.cfg); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to save config: "+err.Error()))
			return
		}

		if params.DeleteFiles && spec.Workspace != "" {
			ws := config.ExpandHome(spec.Workspace)
			os.RemoveAll(ws)
		}
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"ok":              true,
		"agentId":         params.AgentID,
		"removedBindings": removedBindings,
	}))
}

// --- agents.files.list ---
// Matching TS src/gateway/server-methods/agents.ts:399-422

// allowedAgentFiles is the list of files exposed via agents.files.* RPCs.
var allowedAgentFiles = []string{
	bootstrap.AgentsFile, bootstrap.SoulFile, bootstrap.ToolsFile, bootstrap.IdentityFile,
	bootstrap.UserFile, bootstrap.HeartbeatFile, bootstrap.BootstrapFile, bootstrap.MemoryJSONFile,
}

func (m *AgentsMethods) handleFilesList(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params agentParams
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}
	if params.AgentID == "" {
		params.AgentID = "default"
	}

	if m.isManaged && m.agentStore != nil {
		// --- Managed mode: list from DB ---
		ctx := context.Background()
		ag, err := m.agentStore.GetByKey(ctx, params.AgentID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, "agent not found: "+params.AgentID))
			return
		}

		dbFiles, err := m.agentStore.GetAgentContextFiles(ctx, ag.ID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to list files: "+err.Error()))
			return
		}

		// Build a map for quick lookup
		dbMap := make(map[string]store.AgentContextFileData, len(dbFiles))
		for _, f := range dbFiles {
			dbMap[f.FileName] = f
		}

		files := make([]map[string]interface{}, 0, len(allowedAgentFiles))
		for _, name := range allowedAgentFiles {
			if f, ok := dbMap[name]; ok {
				files = append(files, map[string]interface{}{
					"name":    name,
					"missing": false,
					"size":    len(f.Content),
				})
			} else {
				files = append(files, map[string]interface{}{
					"name":    name,
					"missing": true,
				})
			}
		}

		client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
			"agentId": params.AgentID,
			"files":   files,
		}))
		return
	}

	// --- Standalone mode: filesystem ---
	ws := m.resolveWorkspace(params.AgentID)
	files := make([]map[string]interface{}, 0, len(allowedAgentFiles))

	for _, name := range allowedAgentFiles {
		p := filepath.Join(ws, name)
		info, err := os.Stat(p)
		if err != nil {
			files = append(files, map[string]interface{}{
				"name":    name,
				"path":    p,
				"missing": true,
			})
		} else {
			files = append(files, map[string]interface{}{
				"name":        name,
				"path":        p,
				"missing":     false,
				"size":        info.Size(),
				"updatedAtMs": info.ModTime().UnixMilli(),
			})
		}
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"agentId":   params.AgentID,
		"workspace": ws,
		"files":     files,
	}))
}

// --- agents.files.get ---
// Matching TS src/gateway/server-methods/agents.ts:423-473

func (m *AgentsMethods) handleFilesGet(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params struct {
		AgentID string `json:"agentId"`
		Name    string `json:"name"`
	}
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}
	if params.AgentID == "" {
		params.AgentID = "default"
	}
	if params.Name == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "name is required"))
		return
	}
	if !isAllowedFile(params.Name) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "file not allowed: "+params.Name))
		return
	}

	if m.isManaged && m.agentStore != nil {
		// --- Managed mode: read from DB ---
		ctx := context.Background()
		ag, err := m.agentStore.GetByKey(ctx, params.AgentID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, "agent not found: "+params.AgentID))
			return
		}

		dbFiles, err := m.agentStore.GetAgentContextFiles(ctx, ag.ID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to get files: "+err.Error()))
			return
		}

		for _, f := range dbFiles {
			if f.FileName == params.Name {
				client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
					"agentId": params.AgentID,
					"file": map[string]interface{}{
						"name":    params.Name,
						"missing": false,
						"size":    len(f.Content),
						"content": f.Content,
					},
				}))
				return
			}
		}

		// File not found in DB
		client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
			"agentId": params.AgentID,
			"file": map[string]interface{}{
				"name":    params.Name,
				"missing": true,
			},
		}))
		return
	}

	// --- Standalone mode: filesystem ---
	ws := m.resolveWorkspace(params.AgentID)
	p := filepath.Join(ws, params.Name)

	info, err := os.Stat(p)
	if err != nil {
		client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
			"agentId":   params.AgentID,
			"workspace": ws,
			"file": map[string]interface{}{
				"name":    params.Name,
				"path":    p,
				"missing": true,
			},
		}))
		return
	}

	content, _ := os.ReadFile(p)
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"agentId":   params.AgentID,
		"workspace": ws,
		"file": map[string]interface{}{
			"name":        params.Name,
			"path":        p,
			"missing":     false,
			"size":        info.Size(),
			"updatedAtMs": info.ModTime().UnixMilli(),
			"content":     string(content),
		},
	}))
}

// --- agents.files.set ---
// Matching TS src/gateway/server-methods/agents.ts:474-515

func (m *AgentsMethods) handleFilesSet(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params struct {
		AgentID string `json:"agentId"`
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}
	if params.AgentID == "" {
		params.AgentID = "default"
	}
	if params.Name == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "name is required"))
		return
	}
	if !isAllowedFile(params.Name) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "file not allowed: "+params.Name))
		return
	}

	if m.isManaged && m.agentStore != nil {
		// --- Managed mode: write to DB ---
		ctx := context.Background()
		ag, err := m.agentStore.GetByKey(ctx, params.AgentID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, "agent not found: "+params.AgentID))
			return
		}

		if err := m.agentStore.SetAgentContextFile(ctx, ag.ID, params.Name, params.Content); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to write file: "+err.Error()))
			return
		}

		// Invalidate agent cache so new bootstrap content takes effect
		m.agents.InvalidateAgent(params.AgentID)

		client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
			"agentId": params.AgentID,
			"file": map[string]interface{}{
				"name":    params.Name,
				"missing": false,
				"size":    len(params.Content),
				"content": params.Content,
			},
		}))
		return
	}

	// --- Standalone mode: filesystem ---
	ws := m.resolveWorkspace(params.AgentID)
	os.MkdirAll(ws, 0755)
	p := filepath.Join(ws, params.Name)

	if err := os.WriteFile(p, []byte(params.Content), 0644); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to write file: "+err.Error()))
		return
	}

	info, _ := os.Stat(p)
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"agentId":   params.AgentID,
		"workspace": ws,
		"file": map[string]interface{}{
			"name":        params.Name,
			"path":        p,
			"missing":     false,
			"size":        info.Size(),
			"updatedAtMs": info.ModTime().UnixMilli(),
			"content":     params.Content,
		},
	}))
}

// --- agent.identity.get ---
// Matching TS src/gateway/server-methods/agent.ts:601-643

func (m *AgentsMethods) handleIdentityGet(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params struct {
		AgentID    string `json:"agentId"`
		SessionKey string `json:"sessionKey"`
	}
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}
	if params.AgentID == "" {
		// Try to extract from sessionKey: "agent:{agentId}:..."
		if params.SessionKey != "" {
			parts := strings.SplitN(params.SessionKey, ":", 3)
			if len(parts) >= 2 {
				params.AgentID = parts[1]
			}
		}
		if params.AgentID == "" {
			params.AgentID = "default"
		}
	}

	result := map[string]interface{}{
		"agentId": params.AgentID,
	}

	if m.isManaged && m.agentStore != nil {
		// --- Managed mode: read identity from DB ---
		ctx := context.Background()
		ag, err := m.agentStore.GetByKey(ctx, params.AgentID)
		if err == nil {
			result["name"] = ag.DisplayName

			// Parse IDENTITY.md from DB bootstrap
			dbFiles, _ := m.agentStore.GetAgentContextFiles(ctx, ag.ID)
			for _, f := range dbFiles {
				if f.FileName == "IDENTITY.md" {
					if identity := parseIdentityContent(f.Content); identity != nil {
						if identity["Name"] != "" {
							result["name"] = identity["Name"]
						}
						if identity["Emoji"] != "" {
							result["emoji"] = identity["Emoji"]
						}
						if identity["Avatar"] != "" {
							result["avatar"] = identity["Avatar"]
						}
						if identity["Description"] != "" {
							result["description"] = identity["Description"]
						}
					}
					break
				}
			}
		}
	} else {
		// --- Standalone mode: config + filesystem ---
		result["name"] = m.cfg.ResolveDisplayName(params.AgentID)

		if spec, ok := m.cfg.Agents.List[params.AgentID]; ok && spec.Identity != nil {
			if spec.Identity.Emoji != "" {
				result["emoji"] = spec.Identity.Emoji
			}
			if spec.Identity.Name != "" {
				result["name"] = spec.Identity.Name
			}
		}

		ws := m.resolveWorkspace(params.AgentID)
		identityPath := filepath.Join(ws, "IDENTITY.md")
		if identity := parseIdentityFile(identityPath); identity != nil {
			if identity["Name"] != "" {
				result["name"] = identity["Name"]
			}
			if identity["Emoji"] != "" {
				result["emoji"] = identity["Emoji"]
			}
			if identity["Avatar"] != "" {
				result["avatar"] = identity["Avatar"]
			}
			if identity["Description"] != "" {
				result["description"] = identity["Description"]
			}
		}
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, result))
}

// --- Helpers ---

func (m *AgentsMethods) resolveWorkspace(agentID string) string {
	if spec, ok := m.cfg.Agents.List[agentID]; ok && spec.Workspace != "" {
		return config.ExpandHome(spec.Workspace)
	}
	return config.ExpandHome(m.cfg.Agents.Defaults.Workspace)
}

func isAllowedFile(name string) bool {
	for _, f := range allowedAgentFiles {
		if f == name {
			return true
		}
	}
	return false
}

// parseIdentityContent parses IDENTITY.md content string and extracts Key: Value fields.
func parseIdentityContent(content string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			if val != "" {
				result[key] = val
			}
		}
	}
	return result
}

// parseIdentityFile reads IDENTITY.md and extracts Key: Value fields.
func parseIdentityFile(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			if val != "" {
				result[key] = val
			}
		}
	}
	return result
}

// buildIdentityContent creates the content for IDENTITY.md from fields.
func buildIdentityContent(name, emoji, avatar string) string {
	var lines []string
	lines = append(lines, "# Identity")
	if name != "" {
		lines = append(lines, "Name: "+name)
	}
	if emoji != "" {
		lines = append(lines, "Emoji: "+emoji)
	}
	if avatar != "" {
		lines = append(lines, "Avatar: "+avatar)
	}
	return strings.Join(lines, "\n") + "\n"
}

// appendIdentityFields appends Name/Emoji/Avatar to IDENTITY.md.
func appendIdentityFields(path string, name, emoji, avatar string) {
	var lines []string
	if name != "" {
		lines = append(lines, "Name: "+name)
	}
	if emoji != "" {
		lines = append(lines, "Emoji: "+emoji)
	}
	if avatar != "" {
		lines = append(lines, "Avatar: "+avatar)
	}
	if len(lines) == 0 {
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString("\n" + strings.Join(lines, "\n") + "\n")
}
