package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGTeamStore implements store.TeamStore backed by Postgres.
type PGTeamStore struct {
	db *sql.DB
}

func NewPGTeamStore(db *sql.DB) *PGTeamStore {
	return &PGTeamStore{db: db}
}

// --- Column constants ---

const teamSelectCols = `id, name, lead_agent_id, description, status, settings, created_by, created_at, updated_at`

const taskSelectCols = `id, team_id, subject, description, status, owner_agent_id, blocked_by, priority, result, created_at, updated_at`

const messageSelectCols = `id, team_id, from_agent_id, to_agent_id, content, message_type, read, created_at`

// ============================================================
// Team CRUD
// ============================================================

func (s *PGTeamStore) CreateTeam(ctx context.Context, team *store.TeamData) error {
	if team.ID == uuid.Nil {
		team.ID = store.GenNewID()
	}
	now := time.Now()
	team.CreatedAt = now
	team.UpdatedAt = now

	settings := team.Settings
	if len(settings) == 0 {
		settings = json.RawMessage(`{}`)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_teams (id, name, lead_agent_id, description, status, settings, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		team.ID, team.Name, team.LeadAgentID, team.Description,
		team.Status, settings, team.CreatedBy, now, now,
	)
	return err
}

func (s *PGTeamStore) GetTeam(ctx context.Context, teamID uuid.UUID) (*store.TeamData, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+teamSelectCols+` FROM agent_teams WHERE id = $1`, teamID)
	return scanTeamRow(row)
}

func (s *PGTeamStore) DeleteTeam(ctx context.Context, teamID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agent_teams WHERE id = $1`, teamID)
	return err
}

func (s *PGTeamStore) ListTeams(ctx context.Context) ([]store.TeamData, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.name, t.lead_agent_id, t.description, t.status, t.settings, t.created_by, t.created_at, t.updated_at,
		 COALESCE(a.agent_key, '') AS lead_agent_key
		 FROM agent_teams t
		 LEFT JOIN agents a ON a.id = t.lead_agent_id
		 ORDER BY t.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []store.TeamData
	for rows.Next() {
		var d store.TeamData
		var desc sql.NullString
		if err := rows.Scan(
			&d.ID, &d.Name, &d.LeadAgentID, &desc, &d.Status,
			&d.Settings, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt,
			&d.LeadAgentKey,
		); err != nil {
			return nil, err
		}
		if desc.Valid {
			d.Description = desc.String
		}
		teams = append(teams, d)
	}
	return teams, rows.Err()
}

// ============================================================
// Members
// ============================================================

func (s *PGTeamStore) AddMember(ctx context.Context, teamID, agentID uuid.UUID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_team_members (team_id, agent_id, role, joined_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (team_id, agent_id) DO UPDATE SET role = EXCLUDED.role`,
		teamID, agentID, role, time.Now(),
	)
	return err
}

func (s *PGTeamStore) RemoveMember(ctx context.Context, teamID, agentID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM agent_team_members WHERE team_id = $1 AND agent_id = $2`,
		teamID, agentID,
	)
	return err
}

func (s *PGTeamStore) ListMembers(ctx context.Context, teamID uuid.UUID) ([]store.TeamMemberData, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.team_id, m.agent_id, m.role, m.joined_at,
		 COALESCE(a.agent_key, '') AS agent_key,
		 COALESCE(a.display_name, '') AS display_name,
		 COALESCE(a.frontmatter, '') AS frontmatter
		 FROM agent_team_members m
		 JOIN agents a ON a.id = m.agent_id
		 WHERE m.team_id = $1
		 ORDER BY m.joined_at`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []store.TeamMemberData
	for rows.Next() {
		var d store.TeamMemberData
		if err := rows.Scan(
			&d.TeamID, &d.AgentID, &d.Role, &d.JoinedAt,
			&d.AgentKey, &d.DisplayName, &d.Frontmatter,
		); err != nil {
			return nil, err
		}
		members = append(members, d)
	}
	return members, rows.Err()
}

func (s *PGTeamStore) GetTeamForAgent(ctx context.Context, agentID uuid.UUID) (*store.TeamData, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT t.id, t.name, t.lead_agent_id, t.description, t.status, t.settings, t.created_by, t.created_at, t.updated_at
		 FROM agent_teams t
		 JOIN agent_team_members m ON m.team_id = t.id
		 WHERE m.agent_id = $1 AND t.status = $2
		 LIMIT 1`, agentID, store.TeamStatusActive)

	d, err := scanTeamRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// ============================================================
// Tasks
// ============================================================

func (s *PGTeamStore) CreateTask(ctx context.Context, task *store.TeamTaskData) error {
	if task.ID == uuid.Nil {
		task.ID = store.GenNewID()
	}
	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO team_tasks (id, team_id, subject, description, status, owner_agent_id, blocked_by, priority, result, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		task.ID, task.TeamID, task.Subject, task.Description,
		task.Status, task.OwnerAgentID, pq.Array(task.BlockedBy),
		task.Priority, task.Result, now, now,
	)
	return err
}

func (s *PGTeamStore) UpdateTask(ctx context.Context, taskID uuid.UUID, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	updates["updated_at"] = time.Now()
	return execMapUpdate(ctx, s.db, "team_tasks", taskID, updates)
}

func (s *PGTeamStore) ListTasks(ctx context.Context, teamID uuid.UUID, orderBy string) ([]store.TeamTaskData, error) {
	orderClause := "t.priority DESC, t.created_at"
	if orderBy == "newest" {
		orderClause = "t.created_at DESC"
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.team_id, t.subject, t.description, t.status, t.owner_agent_id, t.blocked_by, t.priority, t.result, t.created_at, t.updated_at,
		 COALESCE(a.agent_key, '') AS owner_agent_key
		 FROM team_tasks t
		 LEFT JOIN agents a ON a.id = t.owner_agent_id
		 WHERE t.team_id = $1
		 ORDER BY `+orderClause, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTaskRowsJoined(rows)
}

func (s *PGTeamStore) ClaimTask(ctx context.Context, taskID, agentID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE team_tasks SET status = $1, owner_agent_id = $2, updated_at = $3
		 WHERE id = $4 AND status = $5 AND owner_agent_id IS NULL`,
		store.TeamTaskStatusInProgress, agentID, time.Now(),
		taskID, store.TeamTaskStatusPending,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("task not available for claiming (already claimed or not pending)")
	}
	return nil
}

func (s *PGTeamStore) CompleteTask(ctx context.Context, taskID uuid.UUID, result string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Mark task as completed (must be in_progress — use ClaimTask first)
	res, err := tx.ExecContext(ctx,
		`UPDATE team_tasks SET status = $1, result = $2, updated_at = $3
		 WHERE id = $4 AND status = $5`,
		store.TeamTaskStatusCompleted, result, time.Now(),
		taskID, store.TeamTaskStatusInProgress,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("task not in progress or not found")
	}

	// Unblock dependent tasks: remove this taskID from their blocked_by arrays.
	// Tasks with empty blocked_by after removal become claimable.
	_, err = tx.ExecContext(ctx,
		`UPDATE team_tasks SET blocked_by = array_remove(blocked_by, $1), updated_at = $2
		 WHERE $1 = ANY(blocked_by)`,
		taskID, time.Now(),
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ============================================================
// Messages
// ============================================================

func (s *PGTeamStore) SendMessage(ctx context.Context, msg *store.TeamMessageData) error {
	if msg.ID == uuid.Nil {
		msg.ID = store.GenNewID()
	}
	msg.CreatedAt = time.Now()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO team_messages (id, team_id, from_agent_id, to_agent_id, content, message_type, read, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		msg.ID, msg.TeamID, msg.FromAgentID, msg.ToAgentID,
		msg.Content, msg.MessageType, false, msg.CreatedAt,
	)
	return err
}

func (s *PGTeamStore) GetUnread(ctx context.Context, teamID, agentID uuid.UUID) ([]store.TeamMessageData, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.team_id, m.from_agent_id, m.to_agent_id, m.content, m.message_type, m.read, m.created_at,
		 COALESCE(fa.agent_key, '') AS from_agent_key,
		 COALESCE(ta.agent_key, '') AS to_agent_key
		 FROM team_messages m
		 LEFT JOIN agents fa ON fa.id = m.from_agent_id
		 LEFT JOIN agents ta ON ta.id = m.to_agent_id
		 WHERE m.team_id = $1 AND (m.to_agent_id = $2 OR m.to_agent_id IS NULL) AND m.read = false
		 ORDER BY m.created_at`, teamID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessageRowsJoined(rows)
}

func (s *PGTeamStore) MarkRead(ctx context.Context, messageID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE team_messages SET read = true WHERE id = $1`, messageID)
	return err
}

// ============================================================
// Scan helpers
// ============================================================

func scanTeamRow(row *sql.Row) (*store.TeamData, error) {
	var d store.TeamData
	var desc sql.NullString
	err := row.Scan(
		&d.ID, &d.Name, &d.LeadAgentID, &desc, &d.Status,
		&d.Settings, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if desc.Valid {
		d.Description = desc.String
	}
	return &d, nil
}

func scanTaskRowsJoined(rows *sql.Rows) ([]store.TeamTaskData, error) {
	var tasks []store.TeamTaskData
	for rows.Next() {
		var d store.TeamTaskData
		var desc, result sql.NullString
		var ownerID *uuid.UUID
		var blockedBy []uuid.UUID
		if err := rows.Scan(
			&d.ID, &d.TeamID, &d.Subject, &desc, &d.Status,
			&ownerID, pq.Array(&blockedBy), &d.Priority, &result,
			&d.CreatedAt, &d.UpdatedAt,
			&d.OwnerAgentKey,
		); err != nil {
			return nil, err
		}
		if desc.Valid {
			d.Description = desc.String
		}
		if result.Valid {
			d.Result = &result.String
		}
		d.OwnerAgentID = ownerID
		d.BlockedBy = blockedBy
		tasks = append(tasks, d)
	}
	return tasks, rows.Err()
}

func scanMessageRowsJoined(rows *sql.Rows) ([]store.TeamMessageData, error) {
	var messages []store.TeamMessageData
	for rows.Next() {
		var d store.TeamMessageData
		var toAgentID *uuid.UUID
		if err := rows.Scan(
			&d.ID, &d.TeamID, &d.FromAgentID, &toAgentID,
			&d.Content, &d.MessageType, &d.Read, &d.CreatedAt,
			&d.FromAgentKey, &d.ToAgentKey,
		); err != nil {
			return nil, err
		}
		d.ToAgentID = toAgentID
		messages = append(messages, d)
	}
	return messages, rows.Err()
}
