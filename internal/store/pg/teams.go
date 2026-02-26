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

func (s *PGTeamStore) ListTasks(ctx context.Context, teamID uuid.UUID, orderBy string, statusFilter string) ([]store.TeamTaskData, error) {
	orderClause := "t.priority DESC, t.created_at"
	if orderBy == "newest" {
		orderClause = "t.created_at DESC"
	}

	statusWhere := "AND t.status != 'completed'" // default: active only
	switch statusFilter {
	case store.TeamTaskFilterAll:
		statusWhere = ""
	case store.TeamTaskFilterCompleted:
		statusWhere = "AND t.status = 'completed'"
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.team_id, t.subject, t.description, t.status, t.owner_agent_id, t.blocked_by, t.priority, t.result, t.created_at, t.updated_at,
		 COALESCE(a.agent_key, '') AS owner_agent_key
		 FROM team_tasks t
		 LEFT JOIN agents a ON a.id = t.owner_agent_id
		 WHERE t.team_id = $1 `+statusWhere+`
		 ORDER BY `+orderClause, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTaskRowsJoined(rows)
}

func (s *PGTeamStore) GetTask(ctx context.Context, taskID uuid.UUID) (*store.TeamTaskData, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.team_id, t.subject, t.description, t.status, t.owner_agent_id, t.blocked_by, t.priority, t.result, t.created_at, t.updated_at,
		 COALESCE(a.agent_key, '') AS owner_agent_key
		 FROM team_tasks t
		 LEFT JOIN agents a ON a.id = t.owner_agent_id
		 WHERE t.id = $1`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks, err := scanTaskRowsJoined(rows)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("task not found")
	}
	return &tasks[0], nil
}

func (s *PGTeamStore) SearchTasks(ctx context.Context, teamID uuid.UUID, query string, limit int) ([]store.TeamTaskData, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.team_id, t.subject, t.description, t.status, t.owner_agent_id, t.blocked_by, t.priority, t.result, t.created_at, t.updated_at,
		 COALESCE(a.agent_key, '') AS owner_agent_key
		 FROM team_tasks t
		 LEFT JOIN agents a ON a.id = t.owner_agent_id
		 WHERE t.team_id = $1 AND t.tsv @@ plainto_tsquery('simple', $2)
		 ORDER BY ts_rank(t.tsv, plainto_tsquery('simple', $2)) DESC
		 LIMIT $3`, teamID, query, limit)
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
// Delegation History
// ============================================================

func (s *PGTeamStore) SaveDelegationHistory(ctx context.Context, record *store.DelegationHistoryData) error {
	if record.ID == uuid.Nil {
		record.ID = store.GenNewID()
	}
	now := time.Now()
	record.CreatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO delegation_history (id, source_agent_id, target_agent_id, team_id, team_task_id, user_id, task, mode, status, result, error, iterations, trace_id, duration_ms, created_at, completed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		record.ID, record.SourceAgentID, record.TargetAgentID,
		record.TeamID, record.TeamTaskID,
		record.UserID, record.Task, record.Mode, record.Status,
		record.Result, record.Error, record.Iterations,
		record.TraceID, record.DurationMS, now, record.CompletedAt,
	)
	return err
}

func (s *PGTeamStore) ListDelegationHistory(ctx context.Context, opts store.DelegationHistoryListOpts) ([]store.DelegationHistoryData, int, error) {
	where := "WHERE 1=1"
	args := []any{}
	argN := 0

	nextArg := func(v any) string {
		argN++
		args = append(args, v)
		return fmt.Sprintf("$%d", argN)
	}

	if opts.SourceAgentID != nil {
		where += " AND d.source_agent_id = " + nextArg(*opts.SourceAgentID)
	}
	if opts.TargetAgentID != nil {
		where += " AND d.target_agent_id = " + nextArg(*opts.TargetAgentID)
	}
	if opts.TeamID != nil {
		where += " AND d.team_id = " + nextArg(*opts.TeamID)
	}
	if opts.UserID != "" {
		where += " AND d.user_id = " + nextArg(opts.UserID)
	}
	if opts.Status != "" {
		where += " AND d.status = " + nextArg(opts.Status)
	}

	// Count total
	var total int
	countSQL := "SELECT COUNT(*) FROM delegation_history d " + where
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch rows
	limit := opts.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(
		`SELECT d.id, d.source_agent_id, d.target_agent_id, d.team_id, d.team_task_id,
		 d.user_id, d.task, d.mode, d.status, d.result, d.error, d.iterations,
		 d.trace_id, d.duration_ms, d.created_at, d.completed_at,
		 COALESCE(sa.agent_key, '') AS source_agent_key,
		 COALESCE(ta.agent_key, '') AS target_agent_key
		 FROM delegation_history d
		 LEFT JOIN agents sa ON sa.id = d.source_agent_id
		 LEFT JOIN agents ta ON ta.id = d.target_agent_id
		 %s
		 ORDER BY d.created_at DESC
		 LIMIT %s OFFSET %s`,
		where, nextArg(limit), nextArg(offset))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []store.DelegationHistoryData
	for rows.Next() {
		var d store.DelegationHistoryData
		var result, errStr sql.NullString
		var completedAt sql.NullTime
		if err := rows.Scan(
			&d.ID, &d.SourceAgentID, &d.TargetAgentID, &d.TeamID, &d.TeamTaskID,
			&d.UserID, &d.Task, &d.Mode, &d.Status, &result, &errStr, &d.Iterations,
			&d.TraceID, &d.DurationMS, &d.CreatedAt, &completedAt,
			&d.SourceAgentKey, &d.TargetAgentKey,
		); err != nil {
			return nil, 0, err
		}
		if result.Valid {
			d.Result = &result.String
		}
		if errStr.Valid {
			d.Error = &errStr.String
		}
		if completedAt.Valid {
			d.CompletedAt = &completedAt.Time
		}
		records = append(records, d)
	}
	return records, total, rows.Err()
}

func (s *PGTeamStore) GetDelegationHistory(ctx context.Context, id uuid.UUID) (*store.DelegationHistoryData, error) {
	var d store.DelegationHistoryData
	var result, errStr sql.NullString
	var completedAt sql.NullTime

	err := s.db.QueryRowContext(ctx,
		`SELECT d.id, d.source_agent_id, d.target_agent_id, d.team_id, d.team_task_id,
		 d.user_id, d.task, d.mode, d.status, d.result, d.error, d.iterations,
		 d.trace_id, d.duration_ms, d.created_at, d.completed_at,
		 COALESCE(sa.agent_key, '') AS source_agent_key,
		 COALESCE(ta.agent_key, '') AS target_agent_key
		 FROM delegation_history d
		 LEFT JOIN agents sa ON sa.id = d.source_agent_id
		 LEFT JOIN agents ta ON ta.id = d.target_agent_id
		 WHERE d.id = $1`, id).Scan(
		&d.ID, &d.SourceAgentID, &d.TargetAgentID, &d.TeamID, &d.TeamTaskID,
		&d.UserID, &d.Task, &d.Mode, &d.Status, &result, &errStr, &d.Iterations,
		&d.TraceID, &d.DurationMS, &d.CreatedAt, &completedAt,
		&d.SourceAgentKey, &d.TargetAgentKey,
	)
	if err != nil {
		return nil, err
	}
	if result.Valid {
		d.Result = &result.String
	}
	if errStr.Valid {
		d.Error = &errStr.String
	}
	if completedAt.Valid {
		d.CompletedAt = &completedAt.Time
	}
	return &d, nil
}

// ============================================================
// Handoff routing
// ============================================================

func (s *PGTeamStore) SetHandoffRoute(ctx context.Context, route *store.HandoffRouteData) error {
	if route.ID == uuid.Nil {
		route.ID = store.GenNewID()
	}
	route.CreatedAt = time.Now()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO handoff_routes (id, channel, chat_id, from_agent_key, to_agent_key, reason, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (channel, chat_id)
		 DO UPDATE SET to_agent_key = EXCLUDED.to_agent_key, from_agent_key = EXCLUDED.from_agent_key,
		               reason = EXCLUDED.reason, created_by = EXCLUDED.created_by, created_at = EXCLUDED.created_at`,
		route.ID, route.Channel, route.ChatID, route.FromAgentKey, route.ToAgentKey,
		route.Reason, route.CreatedBy, route.CreatedAt,
	)
	return err
}

func (s *PGTeamStore) GetHandoffRoute(ctx context.Context, channel, chatID string) (*store.HandoffRouteData, error) {
	var d store.HandoffRouteData
	err := s.db.QueryRowContext(ctx,
		`SELECT id, channel, chat_id, from_agent_key, to_agent_key, reason, created_by, created_at
		 FROM handoff_routes WHERE channel = $1 AND chat_id = $2`,
		channel, chatID).Scan(
		&d.ID, &d.Channel, &d.ChatID, &d.FromAgentKey, &d.ToAgentKey,
		&d.Reason, &d.CreatedBy, &d.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *PGTeamStore) ClearHandoffRoute(ctx context.Context, channel, chatID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM handoff_routes WHERE channel = $1 AND chat_id = $2`,
		channel, chatID)
	return err
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
