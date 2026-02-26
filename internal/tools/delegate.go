package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tracing"
)

// teamTaskIDRe extracts a team task UUID from delegate messages.
// Matches patterns like "task id 019c953d-06b3-..." or "task_id: 019c953d-06b3-..."
var teamTaskIDRe = regexp.MustCompile(`task[_ ]id[:\s]+([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

const defaultMaxDelegationLoad = 5

// DelegationTask tracks an active delegation for concurrency control and cancellation.
type DelegationTask struct {
	ID             string     `json:"id"`
	SourceAgentID  uuid.UUID  `json:"source_agent_id"`
	SourceAgentKey string     `json:"source_agent_key"`
	TargetAgentID  uuid.UUID  `json:"target_agent_id"`
	TargetAgentKey string     `json:"target_agent_key"`
	UserID         string     `json:"user_id"`
	Task           string     `json:"task"`
	Status         string     `json:"status"` // "running", "completed", "failed", "cancelled"
	Mode           string     `json:"mode"`   // "sync" or "async"
	SessionKey     string     `json:"session_key"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`

	// Origin metadata for async announce routing
	OriginChannel  string `json:"-"`
	OriginChatID   string `json:"-"`
	OriginPeerKind string `json:"-"`

	// Trace context for announce linking (same pattern as SubagentTask)
	OriginTraceID    uuid.UUID `json:"-"`
	OriginRootSpanID uuid.UUID `json:"-"`

	// Team task auto-completion
	TeamTaskID uuid.UUID `json:"-"`

	cancelFunc context.CancelFunc `json:"-"`
}

// DelegateOpts configures a single delegation call.
type DelegateOpts struct {
	TargetAgentKey string
	Task           string
	Context        string    // optional extra context
	Mode           string    // "sync" (default) or "async"
	TeamTaskID     uuid.UUID // optional: auto-complete this team task on success
}

// DelegateRunRequest is the request passed to the AgentRunFunc callback.
// Mirrors agent.RunRequest without importing the agent package (avoids import cycle).
type DelegateRunRequest struct {
	SessionKey        string
	Message           string
	UserID            string
	Channel           string
	ChatID            string
	PeerKind          string
	RunID             string
	Stream            bool
	ExtraSystemPrompt string
}

// DelegateRunResult is the result from AgentRunFunc.
type DelegateRunResult struct {
	Content    string
	Iterations int
}

// AgentRunFunc runs an agent by key with the given request.
// This callback is injected from the cmd layer to avoid tools→agent import cycle.
type AgentRunFunc func(ctx context.Context, agentKey string, req DelegateRunRequest) (*DelegateRunResult, error)

// DelegateResult is the outcome of a delegation.
type DelegateResult struct {
	Content      string
	Iterations   int
	DelegationID string // for async: the delegation ID to track/cancel
}

// linkSettings holds per-user restriction rules from agent_links.settings JSONB.
// NOTE: This is NOT the same as other_config.description (summoning prompt).
type linkSettings struct {
	RequireRole string   `json:"require_role"`
	UserAllow   []string `json:"user_allow"`
	UserDeny    []string `json:"user_deny"`
}

// DelegateManager manages inter-agent delegation lifecycle.
// Similar to SubagentManager but delegates to fully-configured named agents.
type DelegateManager struct {
	runAgent     AgentRunFunc
	linkStore    store.AgentLinkStore
	agentStore   store.AgentStore
	teamStore    store.TeamStore     // optional: enables auto-complete of team tasks
	sessionStore store.SessionStore  // optional: enables session cleanup
	msgBus       *bus.MessageBus     // for event broadcast + async announce (PublishInbound)

	active            sync.Map // delegationID → *DelegationTask
	completedMu       sync.Mutex
	completedSessions []string // session keys pending cleanup
}

// NewDelegateManager creates a new delegation manager.
func NewDelegateManager(
	runAgent AgentRunFunc,
	linkStore store.AgentLinkStore,
	agentStore store.AgentStore,
	msgBus *bus.MessageBus,
) *DelegateManager {
	return &DelegateManager{
		runAgent:   runAgent,
		linkStore:  linkStore,
		agentStore: agentStore,
		msgBus:     msgBus,
	}
}

// SetTeamStore enables auto-completion of team tasks on delegation success.
func (dm *DelegateManager) SetTeamStore(ts store.TeamStore) {
	dm.teamStore = ts
}

// SetSessionStore enables session cleanup after team tasks complete.
func (dm *DelegateManager) SetSessionStore(ss store.SessionStore) {
	dm.sessionStore = ss
}

// Delegate executes a synchronous delegation to another agent.
func (dm *DelegateManager) Delegate(ctx context.Context, opts DelegateOpts) (*DelegateResult, error) {
	task, _, err := dm.prepareDelegation(ctx, opts, "sync")
	if err != nil {
		return nil, err
	}

	dm.active.Store(task.ID, task)
	defer func() {
		now := time.Now()
		task.CompletedAt = &now
		dm.active.Delete(task.ID)
	}()

	message := buildDelegateMessage(opts)
	dm.emitEvent("delegation.started", task)
	slog.Info("delegation started", "id", task.ID, "target", opts.TargetAgentKey, "mode", "sync")

	// Propagate parent trace ID so the delegate trace links back
	delegateCtx := ctx
	if parentTraceID := tracing.TraceIDFromContext(ctx); parentTraceID != uuid.Nil {
		delegateCtx = tracing.WithDelegateParentTraceID(ctx, parentTraceID)
	}

	result, err := dm.runAgent(delegateCtx, opts.TargetAgentKey, dm.buildRunRequest(task, message))
	if err != nil {
		task.Status = "failed"
		dm.emitEvent("delegation.failed", task)
		return nil, fmt.Errorf("delegation to %q failed: %w", opts.TargetAgentKey, err)
	}

	task.Status = "completed"
	dm.emitEvent("delegation.completed", task)
	dm.trackCompleted(task)
	dm.autoCompleteTeamTask(task, result.Content)
	slog.Info("delegation completed", "id", task.ID, "target", opts.TargetAgentKey, "iterations", result.Iterations)

	return &DelegateResult{Content: result.Content, Iterations: result.Iterations, DelegationID: task.ID}, nil
}

// DelegateAsync spawns a delegation in the background and announces the result back.
func (dm *DelegateManager) DelegateAsync(ctx context.Context, opts DelegateOpts) (*DelegateResult, error) {
	task, _, err := dm.prepareDelegation(ctx, opts, "async")
	if err != nil {
		return nil, err
	}

	taskCtx, taskCancel := context.WithCancel(context.Background())
	task.cancelFunc = taskCancel
	dm.active.Store(task.ID, task)

	// Capture parent trace ID before goroutine (ctx.Background() loses it)
	parentTraceID := tracing.TraceIDFromContext(ctx)
	if parentTraceID != uuid.Nil {
		taskCtx = tracing.WithDelegateParentTraceID(taskCtx, parentTraceID)
	}

	message := buildDelegateMessage(opts)
	dm.emitEvent("delegation.started", task)
	slog.Info("delegation started (async)", "id", task.ID, "target", opts.TargetAgentKey)

	runReq := dm.buildRunRequest(task, message)

	go func() {
		defer func() {
			now := time.Now()
			task.CompletedAt = &now
			dm.active.Delete(task.ID)
		}()

		result, runErr := dm.runAgent(taskCtx, opts.TargetAgentKey, runReq)

		// Announce result to parent via message bus
		if dm.msgBus != nil && task.OriginChannel != "" {
			elapsed := time.Since(task.CreatedAt)
			dm.msgBus.PublishInbound(bus.InboundMessage{
				Channel:  "system",
				SenderID: fmt.Sprintf("delegate:%s", task.ID),
				ChatID:   task.OriginChatID,
				Content:  formatDelegateAnnounce(task, result, runErr, elapsed),
				UserID:   task.UserID,
				Metadata: map[string]string{
					"origin_channel":      task.OriginChannel,
					"origin_peer_kind":    task.OriginPeerKind,
					"parent_agent":        task.SourceAgentKey,
					"delegation_id":       task.ID,
					"target_agent":        task.TargetAgentKey,
					"origin_trace_id":     task.OriginTraceID.String(),
					"origin_root_span_id": task.OriginRootSpanID.String(),
				},
			})
		}

		if runErr != nil {
			task.Status = "failed"
			dm.emitEvent("delegation.failed", task)
		} else {
			task.Status = "completed"
			dm.emitEvent("delegation.completed", task)
			dm.trackCompleted(task)
			if result != nil {
				dm.autoCompleteTeamTask(task, result.Content)
			}
		}
		slog.Info("delegation finished (async)", "id", task.ID, "target", task.TargetAgentKey, "status", task.Status)
	}()

	return &DelegateResult{DelegationID: task.ID}, nil
}

// Cancel cancels a running delegation by ID.
func (dm *DelegateManager) Cancel(delegationID string) bool {
	val, ok := dm.active.Load(delegationID)
	if !ok {
		return false
	}
	task := val.(*DelegationTask)
	if task.cancelFunc != nil {
		task.cancelFunc()
	}
	task.Status = "cancelled"
	now := time.Now()
	task.CompletedAt = &now
	dm.active.Delete(delegationID)
	dm.emitEvent("delegation.cancelled", task)
	slog.Info("delegation cancelled", "id", delegationID, "target", task.TargetAgentKey)
	return true
}

// ListActive returns all active delegations for a source agent.
func (dm *DelegateManager) ListActive(sourceAgentID uuid.UUID) []*DelegationTask {
	var tasks []*DelegationTask
	dm.active.Range(func(_, val any) bool {
		t := val.(*DelegationTask)
		if t.SourceAgentID == sourceAgentID && t.Status == "running" {
			tasks = append(tasks, t)
		}
		return true
	})
	return tasks
}

// ActiveCountForLink counts running delegations for a specific source→target pair.
func (dm *DelegateManager) ActiveCountForLink(sourceID, targetID uuid.UUID) int {
	count := 0
	dm.active.Range(func(_, val any) bool {
		t := val.(*DelegationTask)
		if t.SourceAgentID == sourceID && t.TargetAgentID == targetID && t.Status == "running" {
			count++
		}
		return true
	})
	return count
}

// ActiveCountForTarget counts running delegations targeting a specific agent from all sources.
func (dm *DelegateManager) ActiveCountForTarget(targetID uuid.UUID) int {
	count := 0
	dm.active.Range(func(_, val any) bool {
		t := val.(*DelegationTask)
		if t.TargetAgentID == targetID && t.Status == "running" {
			count++
		}
		return true
	})
	return count
}

// --- internal helpers ---

func (dm *DelegateManager) prepareDelegation(ctx context.Context, opts DelegateOpts, mode string) (*DelegationTask, *store.AgentLinkData, error) {
	sourceAgentID := store.AgentIDFromContext(ctx)
	if sourceAgentID == uuid.Nil {
		return nil, nil, fmt.Errorf("delegation requires managed mode (no agent ID in context)")
	}

	sourceAgent, err := dm.agentStore.GetByID(ctx, sourceAgentID)
	if err != nil {
		return nil, nil, fmt.Errorf("source agent not found: %w", err)
	}

	targetAgent, err := dm.agentStore.GetByKey(ctx, opts.TargetAgentKey)
	if err != nil {
		return nil, nil, fmt.Errorf("target agent %q not found", opts.TargetAgentKey)
	}

	link, err := dm.linkStore.GetLinkBetween(ctx, sourceAgentID, targetAgent.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check delegation permission: %w", err)
	}
	if link == nil {
		return nil, nil, fmt.Errorf("no delegation link from this agent to %q. Available targets are listed in AGENTS.md", opts.TargetAgentKey)
	}

	userID := store.UserIDFromContext(ctx)
	if err := checkUserPermission(link.Settings, userID); err != nil {
		return nil, nil, err
	}

	linkCount := dm.ActiveCountForLink(sourceAgentID, targetAgent.ID)
	if link.MaxConcurrent > 0 && linkCount >= link.MaxConcurrent {
		return nil, nil, fmt.Errorf("delegation link to %q is at capacity (%d/%d active). Try again later or handle the task yourself",
			opts.TargetAgentKey, linkCount, link.MaxConcurrent)
	}

	targetCount := dm.ActiveCountForTarget(targetAgent.ID)
	maxLoad := parseMaxDelegationLoad(targetAgent.OtherConfig)
	if targetCount >= maxLoad {
		return nil, nil, fmt.Errorf("agent %q is at capacity (%d/%d active delegations). Either wait and retry, use a different agent, or handle the task yourself",
			opts.TargetAgentKey, targetCount, maxLoad)
	}

	channel := ToolChannelFromCtx(ctx)
	chatID := ToolChatIDFromCtx(ctx)
	peerKind := ToolPeerKindFromCtx(ctx)

	delegationID := uuid.NewString()[:12]
	task := &DelegationTask{
		ID:             delegationID,
		SourceAgentID:  sourceAgentID,
		SourceAgentKey: sourceAgent.AgentKey,
		TargetAgentID:  targetAgent.ID,
		TargetAgentKey: opts.TargetAgentKey,
		UserID:         userID,
		Task:           opts.Task,
		Status:         "running",
		Mode:           mode,
		SessionKey: fmt.Sprintf("delegate:%s:%s:%s",
			sourceAgentID.String()[:8], opts.TargetAgentKey, delegationID),
		CreatedAt:        time.Now(),
		OriginChannel:    channel,
		OriginChatID:     chatID,
		OriginPeerKind:   peerKind,
		OriginTraceID:    tracing.TraceIDFromContext(ctx),
		OriginRootSpanID: tracing.ParentSpanIDFromContext(ctx),
		TeamTaskID:       resolveTeamTaskID(opts),
	}

	return task, link, nil
}

func buildDelegateMessage(opts DelegateOpts) string {
	if opts.Context != "" {
		return fmt.Sprintf("[Additional Context]\n%s\n\n[Task]\n%s", opts.Context, opts.Task)
	}
	return opts.Task
}

func (dm *DelegateManager) buildRunRequest(task *DelegationTask, message string) DelegateRunRequest {
	return DelegateRunRequest{
		SessionKey: task.SessionKey,
		Message:    message,
		UserID:     task.UserID,
		Channel:    "delegate",
		ChatID:     task.OriginChatID,
		PeerKind:   task.OriginPeerKind,
		RunID:      fmt.Sprintf("delegate-%s", task.ID),
		Stream:     false,
		ExtraSystemPrompt: "[Delegation Context]\nYou are handling a delegated task from another agent.\n" +
			"- Focus exclusively on the delegated task below.\n" +
			"- Your complete response will be returned to the requesting agent.\n" +
			"- Do NOT try to communicate with the end user directly.\n" +
			"- Do NOT use your persona name or self-references (e.g. do not say your name). Write factual, neutral content.\n" +
			"- Be concise and deliver actionable results.",
	}
}

func checkUserPermission(settings json.RawMessage, userID string) error {
	if len(settings) == 0 || string(settings) == "{}" {
		return nil
	}
	var s linkSettings
	if json.Unmarshal(settings, &s) != nil {
		return nil // malformed = fail open
	}
	for _, denied := range s.UserDeny {
		if denied == userID {
			return fmt.Errorf("you are not authorized to use this delegation link")
		}
	}
	if len(s.UserAllow) > 0 {
		for _, allowed := range s.UserAllow {
			if allowed == userID {
				return nil
			}
		}
		return fmt.Errorf("you are not authorized to use this delegation link")
	}
	return nil
}

func parseMaxDelegationLoad(otherConfig json.RawMessage) int {
	if len(otherConfig) == 0 {
		return defaultMaxDelegationLoad
	}
	var cfg struct {
		MaxDelegationLoad int `json:"max_delegation_load"`
	}
	if json.Unmarshal(otherConfig, &cfg) != nil || cfg.MaxDelegationLoad <= 0 {
		return defaultMaxDelegationLoad
	}
	return cfg.MaxDelegationLoad
}

// trackCompleted records a delegate session key for deferred cleanup.
func (dm *DelegateManager) trackCompleted(task *DelegationTask) {
	if dm.sessionStore == nil {
		return
	}
	dm.completedMu.Lock()
	dm.completedSessions = append(dm.completedSessions, task.SessionKey)
	dm.completedMu.Unlock()
}

// flushCompletedSessions deletes all tracked delegate sessions.
func (dm *DelegateManager) flushCompletedSessions() {
	if dm.sessionStore == nil {
		return
	}
	dm.completedMu.Lock()
	sessions := dm.completedSessions
	dm.completedSessions = nil
	dm.completedMu.Unlock()

	for _, key := range sessions {
		if err := dm.sessionStore.Delete(key); err != nil {
			slog.Warn("delegate: session cleanup failed", "session", key, "error", err)
		}
	}
	if len(sessions) > 0 {
		slog.Info("delegate: cleaned up sessions", "count", len(sessions))
	}
}

// autoCompleteTeamTask attempts to claim+complete the associated team task.
// Called after a delegation finishes successfully. Errors are logged but not fatal.
// On success, flushes all tracked delegate sessions (task done = context no longer needed).
func (dm *DelegateManager) autoCompleteTeamTask(task *DelegationTask, resultContent string) {
	if dm.teamStore == nil || task.TeamTaskID == uuid.Nil {
		return
	}
	_ = dm.teamStore.ClaimTask(context.Background(), task.TeamTaskID, task.TargetAgentID)
	if err := dm.teamStore.CompleteTask(context.Background(), task.TeamTaskID, resultContent); err != nil {
		slog.Warn("delegate: failed to auto-complete team task",
			"task_id", task.TeamTaskID, "delegation_id", task.ID, "error", err)
	} else {
		slog.Info("delegate: auto-completed team task",
			"task_id", task.TeamTaskID, "delegation_id", task.ID)
		// Task done — flush delegate sessions
		dm.flushCompletedSessions()
	}
}

// resolveTeamTaskID determines the team task ID from explicit opts or regex fallback.
func resolveTeamTaskID(opts DelegateOpts) uuid.UUID {
	if opts.TeamTaskID != uuid.Nil {
		return opts.TeamTaskID
	}
	// Regex fallback: extract from task message
	if match := teamTaskIDRe.FindStringSubmatch(opts.Task); len(match) >= 2 {
		if id, err := uuid.Parse(match[1]); err == nil {
			return id
		}
	}
	return uuid.Nil
}

func (dm *DelegateManager) emitEvent(name string, task *DelegationTask) {
	if dm.msgBus == nil {
		return
	}
	dm.msgBus.Broadcast(bus.Event{
		Name: name,
		Payload: map[string]string{
			"delegation_id": task.ID,
			"source_agent":  task.SourceAgentID.String(),
			"target_agent":  task.TargetAgentKey,
			"user_id":       task.UserID,
			"mode":          task.Mode,
		},
	})
}

func formatDelegateAnnounce(task *DelegationTask, result *DelegateRunResult, err error, elapsed time.Duration) string {
	if err != nil {
		return fmt.Sprintf(
			"[System Message] Delegation to agent %q failed.\n\nError: %s\n\nStats: runtime %s\n\n"+
				"Handle the task yourself or try a different agent.",
			task.TargetAgentKey, err.Error(), elapsed.Round(time.Millisecond))
	}
	return fmt.Sprintf(
		"[System Message] Delegation to agent %q completed.\n\nResult:\n%s\n\nStats: runtime %s, iterations %d\n\n"+
			"Convert the result above into your normal assistant voice and send that user-facing update now. "+
			"Keep internal details private. Reply ONLY: NO_REPLY if this exact result was already delivered to the user.",
		task.TargetAgentKey, result.Content, elapsed.Round(time.Millisecond), result.Iterations)
}
