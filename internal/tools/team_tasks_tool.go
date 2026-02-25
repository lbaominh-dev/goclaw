package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TeamTasksTool exposes the shared team task list to agents.
// Actions: list, create, claim, complete.
type TeamTasksTool struct {
	manager *TeamToolManager
}

func NewTeamTasksTool(manager *TeamToolManager) *TeamTasksTool {
	return &TeamTasksTool{manager: manager}
}

func (t *TeamTasksTool) Name() string { return "team_tasks" }

func (t *TeamTasksTool) Description() string {
	return "Manage the shared team task list. Actions: list (view all tasks), create (add a new task), claim (self-assign a pending task), complete (mark your task as done with a result). See TEAM.md for your team context."
}

func (t *TeamTasksTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "'list', 'create', 'claim', or 'complete'",
			},
			"subject": map[string]interface{}{
				"type":        "string",
				"description": "Task subject (required for action=create)",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Task description (optional, for action=create)",
			},
			"priority": map[string]interface{}{
				"type":        "number",
				"description": "Task priority, higher = more important (optional, for action=create, default 0)",
			},
			"blocked_by": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Task IDs that must complete before this task can be claimed (optional, for action=create)",
			},
			"task_id": map[string]interface{}{
				"type":        "string",
				"description": "Task ID (required for action=claim and action=complete)",
			},
			"result": map[string]interface{}{
				"type":        "string",
				"description": "Task result summary (required for action=complete)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *TeamTasksTool) Execute(ctx context.Context, args map[string]interface{}) *Result {
	action, _ := args["action"].(string)

	switch action {
	case "list":
		return t.executeList(ctx)
	case "create":
		return t.executeCreate(ctx, args)
	case "claim":
		return t.executeClaim(ctx, args)
	case "complete":
		return t.executeComplete(ctx, args)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s (use list, create, claim, or complete)", action))
	}
}

func (t *TeamTasksTool) executeList(ctx context.Context) *Result {
	team, _, err := t.manager.resolveTeam(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}

	tasks, err := t.manager.teamStore.ListTasks(ctx, team.ID, "priority")
	if err != nil {
		return ErrorResult("failed to list tasks: " + err.Error())
	}

	out, _ := json.Marshal(map[string]interface{}{
		"tasks": tasks,
		"count": len(tasks),
	})
	return SilentResult(string(out))
}

func (t *TeamTasksTool) executeCreate(ctx context.Context, args map[string]interface{}) *Result {
	team, _, err := t.manager.resolveTeam(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}

	subject, _ := args["subject"].(string)
	if subject == "" {
		return ErrorResult("subject is required for create action")
	}

	description, _ := args["description"].(string)
	priority := 0
	if p, ok := args["priority"].(float64); ok {
		priority = int(p)
	}

	var blockedBy []uuid.UUID
	if raw, ok := args["blocked_by"].([]interface{}); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				if id, err := uuid.Parse(s); err == nil {
					blockedBy = append(blockedBy, id)
				}
			}
		}
	}

	status := store.TeamTaskStatusPending
	if len(blockedBy) > 0 {
		status = store.TeamTaskStatusBlocked
	}

	task := &store.TeamTaskData{
		TeamID:      team.ID,
		Subject:     subject,
		Description: description,
		Status:      status,
		BlockedBy:   blockedBy,
		Priority:    priority,
	}

	if err := t.manager.teamStore.CreateTask(ctx, task); err != nil {
		return ErrorResult("failed to create task: " + err.Error())
	}

	return NewResult(fmt.Sprintf("Task created: %s (id=%s, status=%s)", subject, task.ID, status))
}

func (t *TeamTasksTool) executeClaim(ctx context.Context, args map[string]interface{}) *Result {
	_, agentID, err := t.manager.resolveTeam(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}

	taskIDStr, _ := args["task_id"].(string)
	if taskIDStr == "" {
		return ErrorResult("task_id is required for claim action")
	}
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		return ErrorResult("invalid task_id")
	}

	if err := t.manager.teamStore.ClaimTask(ctx, taskID, agentID); err != nil {
		return ErrorResult("failed to claim task: " + err.Error())
	}

	return NewResult(fmt.Sprintf("Task %s claimed successfully. It is now in progress.", taskIDStr))
}

func (t *TeamTasksTool) executeComplete(ctx context.Context, args map[string]interface{}) *Result {
	_, agentID, err := t.manager.resolveTeam(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}

	taskIDStr, _ := args["task_id"].(string)
	if taskIDStr == "" {
		return ErrorResult("task_id is required for complete action")
	}
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		return ErrorResult("invalid task_id")
	}

	result, _ := args["result"].(string)
	if result == "" {
		return ErrorResult("result is required for complete action")
	}

	// Auto-claim if the task is still pending (saves an extra tool call).
	// ClaimTask is atomic — only one agent can succeed, others get an error.
	// Ignore claim error: task may already be in_progress (claimed by us or someone else).
	_ = t.manager.teamStore.ClaimTask(ctx, taskID, agentID)

	if err := t.manager.teamStore.CompleteTask(ctx, taskID, result); err != nil {
		return ErrorResult("failed to complete task: " + err.Error())
	}

	return NewResult(fmt.Sprintf("Task %s completed. Dependent tasks have been unblocked.", taskIDStr))
}
