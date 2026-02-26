package methods

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// TeamsMethods handles teams.* RPC methods.
type TeamsMethods struct {
	teamStore   store.TeamStore
	agentStore  store.AgentStore
	linkStore   store.AgentLinkStore // for auto-creating bidirectional links
	agentRouter *agent.Router        // for cache invalidation
}

func NewTeamsMethods(teamStore store.TeamStore, agentStore store.AgentStore, linkStore store.AgentLinkStore, agentRouter *agent.Router) *TeamsMethods {
	return &TeamsMethods{teamStore: teamStore, agentStore: agentStore, linkStore: linkStore, agentRouter: agentRouter}
}

func (m *TeamsMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodTeamsList, m.handleList)
	router.Register(protocol.MethodTeamsCreate, m.handleCreate)
	router.Register(protocol.MethodTeamsGet, m.handleGet)
	router.Register(protocol.MethodTeamsDelete, m.handleDelete)
	router.Register(protocol.MethodTeamsTaskList, m.handleTaskList)
}

// --- List ---

func (m *TeamsMethods) handleList(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if m.teamStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "teams not available (standalone mode)"))
		return
	}

	ctx := context.Background()
	teams, err := m.teamStore.ListTeams(ctx)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"teams": teams,
		"count": len(teams),
	}))
}

// --- Create ---

type teamsCreateParams struct {
	Name        string          `json:"name"`
	Lead        string          `json:"lead"`    // agent key or UUID
	Members     []string        `json:"members"` // agent keys or UUIDs
	Description string          `json:"description"`
	Settings    json.RawMessage `json:"settings"`
}

func (m *TeamsMethods) handleCreate(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if m.teamStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "teams not available (standalone mode)"))
		return
	}

	var params teamsCreateParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	if params.Name == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "name is required"))
		return
	}
	if params.Lead == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "lead is required"))
		return
	}

	// Resolve lead agent
	leadAgent, err := resolveAgentInfo(m.agentStore, params.Lead)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "lead agent: "+err.Error()))
		return
	}

	// Resolve member agents
	var memberAgents []*store.AgentData
	for _, memberKey := range params.Members {
		ag, err := resolveAgentInfo(m.agentStore, memberKey)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "member agent "+memberKey+": "+err.Error()))
			return
		}
		memberAgents = append(memberAgents, ag)
	}

	ctx := context.Background()

	// Create team
	team := &store.TeamData{
		Name:        params.Name,
		LeadAgentID: leadAgent.ID,
		Description: params.Description,
		Status:      store.TeamStatusActive,
		Settings:    params.Settings,
		CreatedBy:   client.UserID(),
	}
	if err := m.teamStore.CreateTeam(ctx, team); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to create team: "+err.Error()))
		return
	}

	// Add lead as member with lead role
	if err := m.teamStore.AddMember(ctx, team.ID, leadAgent.ID, store.TeamRoleLead); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to add lead as member: "+err.Error()))
		return
	}

	// Add members
	for _, ag := range memberAgents {
		if ag.ID == leadAgent.ID {
			continue // lead already added
		}
		if err := m.teamStore.AddMember(ctx, team.ID, ag.ID, store.TeamRoleMember); err != nil {
			slog.Warn("teams.create: failed to add member", "agent", ag.AgentKey, "error", err)
		}
	}

	// Auto-create bidirectional agent_links between all team members.
	// This enables delegation between teammates.
	if m.linkStore != nil {
		allAgents := append([]*store.AgentData{leadAgent}, memberAgents...)
		m.autoCreateTeamLinks(ctx, team.ID, allAgents, client.UserID())
	}

	// Invalidate agent caches so TEAM.md gets injected
	if m.agentRouter != nil {
		m.agentRouter.InvalidateAgent(leadAgent.AgentKey)
		for _, ag := range memberAgents {
			m.agentRouter.InvalidateAgent(ag.AgentKey)
		}
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"team": team,
	}))
}

// --- Get ---

type teamsGetParams struct {
	TeamID string `json:"teamId"`
}

func (m *TeamsMethods) handleGet(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if m.teamStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "teams not available (standalone mode)"))
		return
	}

	var params teamsGetParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	if params.TeamID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "teamId is required"))
		return
	}

	teamID, err := uuid.Parse(params.TeamID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid teamId"))
		return
	}

	ctx := context.Background()
	team, err := m.teamStore.GetTeam(ctx, teamID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}

	members, err := m.teamStore.ListMembers(ctx, teamID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"team":    team,
		"members": members,
	}))
}

// --- Delete ---

type teamsDeleteParams struct {
	TeamID string `json:"teamId"`
}

func (m *TeamsMethods) handleDelete(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if m.teamStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "teams not available (standalone mode)"))
		return
	}

	var params teamsDeleteParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	if params.TeamID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "teamId is required"))
		return
	}

	teamID, err := uuid.Parse(params.TeamID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid teamId"))
		return
	}

	ctx := context.Background()

	// Fetch members before deleting for cache invalidation
	members, _ := m.teamStore.ListMembers(ctx, teamID)

	if err := m.teamStore.DeleteTeam(ctx, teamID); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to delete team: "+err.Error()))
		return
	}

	// Invalidate agent caches
	if m.agentRouter != nil {
		for _, member := range members {
			m.agentRouter.InvalidateAgent(member.AgentKey)
		}
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{"ok": true}))
}

// --- Task List (admin view) ---

type teamsTaskListParams struct {
	TeamID string `json:"teamId"`
}

func (m *TeamsMethods) handleTaskList(_ context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if m.teamStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "teams not available (standalone mode)"))
		return
	}

	var params teamsTaskListParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	if params.TeamID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "teamId is required"))
		return
	}

	teamID, err := uuid.Parse(params.TeamID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid teamId"))
		return
	}

	ctx := context.Background()
	tasks, err := m.teamStore.ListTasks(ctx, teamID, "newest", store.TeamTaskFilterAll)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"tasks": tasks,
		"count": len(tasks),
	}))
}

// --- helpers ---

// autoCreateTeamLinks creates bidirectional agent_links between all team members.
// Silently skips existing links (UNIQUE constraint).
func (m *TeamsMethods) autoCreateTeamLinks(ctx context.Context, teamID uuid.UUID, agents []*store.AgentData, createdBy string) {
	for i := 0; i < len(agents); i++ {
		for j := i + 1; j < len(agents); j++ {
			link := &store.AgentLinkData{
				SourceAgentID: agents[i].ID,
				TargetAgentID: agents[j].ID,
				Direction:     store.LinkDirectionBidirectional,
				TeamID:        &teamID,
				Description:   "auto-created by team",
				MaxConcurrent: 3,
				Status:        store.LinkStatusActive,
				CreatedBy:     createdBy,
			}
			if err := m.linkStore.CreateLink(ctx, link); err != nil {
				slog.Debug("teams: auto-link already exists or failed",
					"source", agents[i].AgentKey, "target", agents[j].AgentKey, "error", err)
			}
		}
	}
}
