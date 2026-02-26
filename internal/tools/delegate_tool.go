package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// DelegateTool is a thin wrapper around DelegateManager.
// Supports actions: delegate (default), cancel, list.
type DelegateTool struct {
	manager *DelegateManager
}

func NewDelegateTool(manager *DelegateManager) *DelegateTool {
	return &DelegateTool{manager: manager}
}

func (t *DelegateTool) Name() string { return "delegate" }

func (t *DelegateTool) Description() string {
	return "Delegate a task to another specialized agent, cancel a running delegation, or list active delegations. The target agent runs with its own identity, tools, and expertise. See AGENTS.md for available agents."
}

func (t *DelegateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "'delegate' (default), 'cancel', or 'list'",
			},
			"agent": map[string]interface{}{
				"type":        "string",
				"description": "Target agent key (required for action=delegate)",
			},
			"task": map[string]interface{}{
				"type":        "string",
				"description": "Task description (required for action=delegate)",
			},
			"context": map[string]interface{}{
				"type":        "string",
				"description": "Optional additional context for the target agent",
			},
			"mode": map[string]interface{}{
				"type":        "string",
				"description": "'sync' (default, blocks until done) or 'async' (returns immediately, result announced later)",
			},
			"delegation_id": map[string]interface{}{
				"type":        "string",
				"description": "Delegation ID to cancel (required for action=cancel)",
			},
			"team_task_id": map[string]interface{}{
				"type":        "string",
				"description": "Team task ID to auto-complete when delegation finishes (optional, for team workflows)",
			},
		},
		"required": []string{},
	}
}

func (t *DelegateTool) Execute(ctx context.Context, args map[string]interface{}) *Result {
	action, _ := args["action"].(string)
	if action == "" {
		action = "delegate"
	}

	switch action {
	case "delegate":
		return t.executeDelegation(ctx, args)
	case "cancel":
		return t.executeCancel(args)
	case "list":
		return t.executeList(ctx)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s (use delegate, cancel, or list)", action))
	}
}

func (t *DelegateTool) executeDelegation(ctx context.Context, args map[string]interface{}) *Result {
	agentKey, _ := args["agent"].(string)
	if agentKey == "" {
		return ErrorResult("agent parameter is required for delegation")
	}
	task, _ := args["task"].(string)
	if task == "" {
		return ErrorResult("task parameter is required for delegation")
	}

	extraContext, _ := args["context"].(string)
	mode, _ := args["mode"].(string)
	if mode == "" {
		mode = "sync"
	}

	var teamTaskID uuid.UUID
	if ttID, _ := args["team_task_id"].(string); ttID != "" {
		teamTaskID, _ = uuid.Parse(ttID)
	}

	opts := DelegateOpts{
		TargetAgentKey: agentKey,
		Task:           task,
		Context:        extraContext,
		Mode:           mode,
		TeamTaskID:     teamTaskID,
	}

	if mode == "async" {
		result, err := t.manager.DelegateAsync(ctx, opts)
		if err != nil {
			return ErrorResult(err.Error())
		}
		forLLM := fmt.Sprintf(`{"status":"accepted","delegation_id":%q,"target":%q,"mode":"async"}
Delegated to %q (async, id=%s). The result will be announced automatically when done — do NOT wait or poll.
Briefly tell the user what you've delegated and to whom. Be friendly and natural.`,
			result.DelegationID, agentKey, agentKey, result.DelegationID)
		return AsyncResult(forLLM)
	}

	// Sync (default)
	result, err := t.manager.Delegate(ctx, opts)
	if err != nil {
		return ErrorResult(err.Error())
	}
	forLLM := fmt.Sprintf(
		"Delegation to %q completed (%d iterations).\n\nResult:\n%s\n\n"+
			"Present the information above to the user in YOUR OWN voice and persona. "+
			"Do NOT adopt the delegate agent's personality, tone, or self-references. "+
			"Rephrase and summarize naturally as yourself.",
		agentKey, result.Iterations, result.Content)
	return NewResult(forLLM)
}

func (t *DelegateTool) executeCancel(args map[string]interface{}) *Result {
	delegationID, _ := args["delegation_id"].(string)
	if delegationID == "" {
		return ErrorResult("delegation_id is required for cancel action")
	}
	if t.manager.Cancel(delegationID) {
		return NewResult(fmt.Sprintf("Delegation %s cancelled.", delegationID))
	}
	return ErrorResult(fmt.Sprintf("delegation %s not found or already completed", delegationID))
}

func (t *DelegateTool) executeList(ctx context.Context) *Result {
	sourceAgentID := store.AgentIDFromContext(ctx)
	tasks := t.manager.ListActive(sourceAgentID)
	if len(tasks) == 0 {
		return SilentResult(`{"delegations":[],"count":0}`)
	}
	out, _ := json.Marshal(map[string]interface{}{
		"delegations": tasks,
		"count":       len(tasks),
	})
	return SilentResult(string(out))
}
