package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Team status constants.
const (
	TeamStatusActive   = "active"
	TeamStatusArchived = "archived"
)

// Team member role constants.
const (
	TeamRoleLead   = "lead"
	TeamRoleMember = "member"
)

// Team task status constants.
const (
	TeamTaskStatusPending    = "pending"
	TeamTaskStatusInProgress = "in_progress"
	TeamTaskStatusCompleted  = "completed"
	TeamTaskStatusBlocked    = "blocked"
)

// Team message type constants.
const (
	TeamMessageTypeChat      = "chat"
	TeamMessageTypeBroadcast = "broadcast"
)

// TeamData represents an agent team.
type TeamData struct {
	BaseModel
	Name        string          `json:"name"`
	LeadAgentID uuid.UUID       `json:"lead_agent_id"`
	Description string          `json:"description,omitempty"`
	Status      string          `json:"status"`
	Settings    json.RawMessage `json:"settings,omitempty"`
	CreatedBy   string          `json:"created_by"`

	// Joined fields (populated by queries that JOIN agents table)
	LeadAgentKey string `json:"lead_agent_key,omitempty"`
}

// TeamMemberData represents a team member.
type TeamMemberData struct {
	TeamID   uuid.UUID `json:"team_id"`
	AgentID  uuid.UUID `json:"agent_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`

	// Joined fields
	AgentKey    string `json:"agent_key,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Frontmatter string `json:"frontmatter,omitempty"`
}

// TeamTaskData represents a task in the team's shared task list.
type TeamTaskData struct {
	BaseModel
	TeamID       uuid.UUID    `json:"team_id"`
	Subject      string       `json:"subject"`
	Description  string       `json:"description,omitempty"`
	Status       string       `json:"status"`
	OwnerAgentID *uuid.UUID   `json:"owner_agent_id,omitempty"`
	BlockedBy    []uuid.UUID  `json:"blocked_by,omitempty"`
	Priority     int          `json:"priority"`
	Result       *string      `json:"result,omitempty"`

	// Joined fields
	OwnerAgentKey string `json:"owner_agent_key,omitempty"`
}

// TeamMessageData represents a message in the team mailbox.
type TeamMessageData struct {
	ID          uuid.UUID  `json:"id"`
	TeamID      uuid.UUID  `json:"team_id"`
	FromAgentID uuid.UUID  `json:"from_agent_id"`
	ToAgentID   *uuid.UUID `json:"to_agent_id,omitempty"`
	Content     string     `json:"content"`
	MessageType string     `json:"message_type"`
	Read        bool       `json:"read"`
	CreatedAt   time.Time  `json:"created_at"`

	// Joined fields
	FromAgentKey string `json:"from_agent_key,omitempty"`
	ToAgentKey   string `json:"to_agent_key,omitempty"`
}

// TeamStore manages agent teams, tasks, and messages.
type TeamStore interface {
	// Team CRUD
	CreateTeam(ctx context.Context, team *TeamData) error
	GetTeam(ctx context.Context, teamID uuid.UUID) (*TeamData, error)
	DeleteTeam(ctx context.Context, teamID uuid.UUID) error
	ListTeams(ctx context.Context) ([]TeamData, error)

	// Members
	AddMember(ctx context.Context, teamID, agentID uuid.UUID, role string) error
	RemoveMember(ctx context.Context, teamID, agentID uuid.UUID) error
	ListMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMemberData, error)

	// GetTeamForAgent returns the team that the given agent belongs to.
	// Returns nil, nil if the agent is not in any team.
	GetTeamForAgent(ctx context.Context, agentID uuid.UUID) (*TeamData, error)

	// Tasks (shared task list)
	CreateTask(ctx context.Context, task *TeamTaskData) error
	UpdateTask(ctx context.Context, taskID uuid.UUID, updates map[string]any) error
	// ListTasks returns tasks for a team. orderBy: "priority" (default) or "newest".
	ListTasks(ctx context.Context, teamID uuid.UUID, orderBy string) ([]TeamTaskData, error)

	// ClaimTask atomically transitions a task from pending to in_progress.
	// Only one agent can claim a given task (row-level lock, race-safe).
	ClaimTask(ctx context.Context, taskID, agentID uuid.UUID) error

	// CompleteTask marks a task as completed and unblocks dependent tasks.
	CompleteTask(ctx context.Context, taskID uuid.UUID, result string) error

	// Messages (mailbox)
	SendMessage(ctx context.Context, msg *TeamMessageData) error
	GetUnread(ctx context.Context, teamID, agentID uuid.UUID) ([]TeamMessageData, error)
	MarkRead(ctx context.Context, messageID uuid.UUID) error
}
