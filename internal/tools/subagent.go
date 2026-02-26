// Package tools provides the subagent system for spawning child agent instances.
//
// Subagents run in background goroutines with restricted tool access.
// Key constraints from OpenClaw spec:
//   - Depth limit: configurable maxSpawnDepth (default 3)
//   - Max children per parent: configurable (default 8)
//   - Auto-archive after configurable TTL (default 30 min)
//   - Tool deny lists: ALWAYS_DENY + LEAF_DENY at max depth
//   - Results announced back to parent via message bus
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tracing"
)

// SubagentConfig configures the subagent system.
type SubagentConfig struct {
	MaxConcurrent       int    // max concurrent subagents (default 4)
	MaxSpawnDepth       int    // max nesting depth (default 3)
	MaxChildrenPerAgent int    // max children per parent (default 8)
	ArchiveAfterMinutes int    // auto-archive completed tasks (default 30)
	Model               string // model override for subagents (empty = inherit)
}

// DefaultSubagentConfig returns sensible defaults matching OpenClaw TS spec.
// TS sources: agent-limits.ts, sessions-spawn-tool.ts, subagent-registry.ts.
func DefaultSubagentConfig() SubagentConfig {
	return SubagentConfig{
		MaxConcurrent:       8,  // TS: DEFAULT_SUBAGENT_MAX_CONCURRENT = 8
		MaxSpawnDepth:       1,  // TS: maxSpawnDepth ?? 1
		MaxChildrenPerAgent: 5,  // TS: maxChildrenPerAgent ?? 5
		ArchiveAfterMinutes: 60, // TS: archiveAfterMinutes ?? 60
	}
}

// Subagent task status constants.
const (
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
	TaskStatusCancelled = "cancelled"
)

// SubagentTask tracks a running or completed subagent.
type SubagentTask struct {
	ID              string `json:"id"`
	ParentID        string `json:"parentId"`
	Task            string `json:"task"`
	Label           string `json:"label"`
	Status          string `json:"status"` // "running", "completed", "failed", "cancelled"
	Result          string `json:"result,omitempty"`
	Depth           int    `json:"depth"`
	Model           string `json:"model,omitempty"`           // model override for this subagent
	OriginChannel   string `json:"originChannel,omitempty"`
	OriginChatID    string `json:"originChatId,omitempty"`
	OriginPeerKind  string `json:"originPeerKind,omitempty"`  // "direct" or "group" (for session key building)
	OriginUserID    string `json:"originUserId,omitempty"`    // parent's userID for per-user scoping propagation
	CreatedAt        int64  `json:"createdAt"`
	CompletedAt      int64  `json:"completedAt,omitempty"`
	OriginTraceID    uuid.UUID `json:"-"` // parent trace for announce linking
	OriginRootSpanID uuid.UUID `json:"-"` // parent agent's root span ID
	cancelFunc       context.CancelFunc `json:"-"` // per-task context cancel
}

// SubagentManager manages the lifecycle of spawned subagents.
type SubagentManager struct {
	mu       sync.RWMutex
	tasks    map[string]*SubagentTask
	config   SubagentConfig
	provider providers.Provider
	model    string
	msgBus   *bus.MessageBus

	// createTools builds a tool registry for subagents (without spawn/subagent tools).
	createTools   func() *Registry
	announceQueue *AnnounceQueue // optional: batches announces with debounce
}

// NewSubagentManager creates a new subagent manager.
func NewSubagentManager(
	provider providers.Provider,
	model string,
	msgBus *bus.MessageBus,
	createTools func() *Registry,
	cfg SubagentConfig,
) *SubagentManager {
	return &SubagentManager{
		tasks:       make(map[string]*SubagentTask),
		config:      cfg,
		provider:    provider,
		model:       model,
		msgBus:      msgBus,
		createTools: createTools,
	}
}

// SetAnnounceQueue sets the announce queue for batched announce delivery.
// If set, runTask() enqueues announces instead of publishing directly.
func (sm *SubagentManager) SetAnnounceQueue(q *AnnounceQueue) {
	sm.announceQueue = q
}

// CountRunningForParent returns the number of running tasks for a parent.
func (sm *SubagentManager) CountRunningForParent(parentID string) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	count := 0
	for _, t := range sm.tasks {
		if t.ParentID == parentID && t.Status == TaskStatusRunning {
			count++
		}
	}
	return count
}

// SubagentDenyAlways is the list of tools always denied to subagents.
var SubagentDenyAlways = []string{
	"gateway",
	"agents_list",
	"whatsapp_login",
	"session_status",
	"cron",
	"memory_search",
	"memory_get",
	"sessions_send",
}

// SubagentDenyLeaf is the additional deny list for subagents at max depth.
var SubagentDenyLeaf = []string{
	"sessions_list",
	"sessions_history",
	"sessions_spawn",
	"spawn",
	"subagent",
}

// Spawn creates a new subagent task that runs asynchronously.
// Returns immediately with a status message. The subagent runs in a goroutine.
// modelOverride optionally overrides the LLM model for this subagent (matching TS sessions-spawn-tool.ts).
func (sm *SubagentManager) Spawn(
	ctx context.Context,
	parentID string,
	depth int,
	task, label, modelOverride string,
	channel, chatID, peerKind string,
	callback AsyncCallback,
) (string, error) {
	sm.mu.Lock()

	// Check depth limit
	if depth >= sm.config.MaxSpawnDepth {
		sm.mu.Unlock()
		return "", fmt.Errorf("spawn depth limit reached (%d/%d)", depth, sm.config.MaxSpawnDepth)
	}

	// Check concurrent limit
	running := 0
	for _, t := range sm.tasks {
		if t.Status == TaskStatusRunning {
			running++
		}
	}
	if running >= sm.config.MaxConcurrent {
		sm.mu.Unlock()
		return "", fmt.Errorf("max concurrent subagents reached (%d/%d)", running, sm.config.MaxConcurrent)
	}

	// Check per-parent children limit
	childCount := 0
	for _, t := range sm.tasks {
		if t.ParentID == parentID {
			childCount++
		}
	}
	if childCount >= sm.config.MaxChildrenPerAgent {
		sm.mu.Unlock()
		return "", fmt.Errorf("max children per agent reached (%d/%d)", childCount, sm.config.MaxChildrenPerAgent)
	}

	id := generateSubagentID()
	if label == "" {
		label = truncate(task, 50)
	}

	subTask := &SubagentTask{
		ID:               id,
		ParentID:         parentID,
		Task:             task,
		Label:            label,
		Status:           "running",
		Depth:            depth + 1,
		Model:            modelOverride,
		OriginChannel:    channel,
		OriginChatID:     chatID,
		OriginPeerKind:   peerKind,
		OriginUserID:     store.UserIDFromContext(ctx),
		OriginTraceID:    tracing.TraceIDFromContext(ctx),
		OriginRootSpanID: tracing.ParentSpanIDFromContext(ctx),
		CreatedAt:        time.Now().UnixMilli(),
	}
	// Create per-task context for real goroutine cancellation
	taskCtx, taskCancel := context.WithCancel(ctx)
	subTask.cancelFunc = taskCancel

	sm.tasks[id] = subTask
	sm.mu.Unlock()

	slog.Info("subagent spawned", "id", id, "parent", parentID, "depth", subTask.Depth, "label", label)

	go sm.runTask(taskCtx, subTask, callback)

	return fmt.Sprintf("Spawned subagent '%s' (id=%s, depth=%d) for task: %s",
		label, id, subTask.Depth, truncate(task, 100)), nil
}

// RunSync executes a subagent task synchronously, blocking until completion.
func (sm *SubagentManager) RunSync(
	ctx context.Context,
	parentID string,
	depth int,
	task, label string,
	channel, chatID string,
) (string, int, error) {
	sm.mu.Lock()

	if depth >= sm.config.MaxSpawnDepth {
		sm.mu.Unlock()
		return "", 0, fmt.Errorf("spawn depth limit reached (%d/%d)", depth, sm.config.MaxSpawnDepth)
	}

	id := generateSubagentID()
	if label == "" {
		label = truncate(task, 50)
	}

	subTask := &SubagentTask{
		ID:               id,
		ParentID:         parentID,
		Task:             task,
		Label:            label,
		Status:           "running",
		Depth:            depth + 1,
		OriginChannel:    channel,
		OriginChatID:     chatID,
		OriginUserID:     store.UserIDFromContext(ctx),
		OriginTraceID:    tracing.TraceIDFromContext(ctx),
		OriginRootSpanID: tracing.ParentSpanIDFromContext(ctx),
		CreatedAt:        time.Now().UnixMilli(),
	}
	sm.tasks[id] = subTask
	sm.mu.Unlock()

	slog.Info("subagent sync started", "id", id, "parent", parentID, "depth", subTask.Depth, "label", label)

	iterations := sm.executeTask(ctx, subTask)

	if subTask.Status == TaskStatusFailed {
		return subTask.Result, iterations, fmt.Errorf("subagent failed: %s", subTask.Result)
	}

	return subTask.Result, iterations, nil
}

// runTask executes the subagent in a goroutine.
func (sm *SubagentManager) runTask(ctx context.Context, task *SubagentTask, callback AsyncCallback) {
	iterations := sm.executeTask(ctx, task)

	// Announce result to parent via bus (matching TS subagent-announce.ts pattern).
	// The announce goes through the parent agent's session so the agent can
	// reformulate the result for the user.
	if sm.msgBus != nil && task.OriginChannel != "" {
		elapsed := time.Since(time.UnixMilli(task.CreatedAt))

		item := AnnounceQueueItem{
			SubagentID: task.ID,
			Label:      task.Label,
			Status:     task.Status,
			Result:     task.Result,
			Runtime:    elapsed,
			Iterations: iterations,
		}
		meta := AnnounceMetadata{
			OriginChannel:    task.OriginChannel,
			OriginChatID:     task.OriginChatID,
			OriginPeerKind:   task.OriginPeerKind,
			OriginUserID:     task.OriginUserID,
			ParentAgent:      task.ParentID,
			OriginTraceID:    task.OriginTraceID.String(),
			OriginRootSpanID: task.OriginRootSpanID.String(),
		}

		if sm.announceQueue != nil {
			// Use batched announce queue (matching TS debounce pattern)
			sessionKey := fmt.Sprintf("announce:%s:%s", task.ParentID, task.OriginChatID)
			sm.announceQueue.Enqueue(sessionKey, item, meta)
		} else {
			// Direct publish (no batching)
			remainingActive := sm.CountRunningForParent(task.ParentID)
			announceContent := FormatBatchedAnnounce([]AnnounceQueueItem{item}, remainingActive)

			sm.msgBus.PublishInbound(bus.InboundMessage{
				Channel:  "system",
				SenderID: fmt.Sprintf("subagent:%s", task.ID),
				ChatID:   task.OriginChatID,
				Content:  announceContent,
				UserID:   task.OriginUserID,
				Metadata: map[string]string{
					"origin_channel":      task.OriginChannel,
					"origin_peer_kind":    task.OriginPeerKind,
					"parent_agent":        task.ParentID,
					"subagent_id":         task.ID,
					"subagent_label":      task.Label,
					"origin_trace_id":     task.OriginTraceID.String(),
					"origin_root_span_id": task.OriginRootSpanID.String(),
				},
			})
		}
	}

	// Call completion callback
	if callback != nil {
		result := NewResult(fmt.Sprintf("Subagent '%s' completed in %d iterations.\n\nResult:\n%s",
			task.Label, iterations, task.Result))
		callback(ctx, result)
	}
}

// executeTask runs the LLM tool loop for a subagent. Returns iteration count.
func (sm *SubagentManager) executeTask(ctx context.Context, task *SubagentTask) int {
	// Tracing: generate a root span ID for this subagent execution.
	// LLM/tool spans will nest under this root span via parent_span_id.
	// The root span itself links to the parent agent's root span (from ctx).
	subRootSpanID := store.GenNewID()
	taskStart := time.Now().UTC()

	// Use a detached context for tracing so spans are emitted even if parent ctx is cancelled.
	// We copy tracing values but remove the cancellation chain.
	traceCtx := context.Background()
	if collector := tracing.CollectorFromContext(ctx); collector != nil {
		traceCtx = tracing.WithCollector(traceCtx, collector)
		traceCtx = tracing.WithTraceID(traceCtx, tracing.TraceIDFromContext(ctx))
		// Keep original parent_span_id (parent agent's root span) for the subagent root span.
		traceCtx = tracing.WithParentSpanID(traceCtx, tracing.ParentSpanIDFromContext(ctx))
	}

	// subCtx overrides parent_span_id so child spans nest under subRootSpanID.
	// traceCtx retains the original parent_span_id for the root subagent span.
	subTraceCtx := tracing.WithParentSpanID(traceCtx, subRootSpanID)

	var model string
	var finalContent string
	iteration := 0

	defer func() {
		sm.mu.Lock()
		task.CompletedAt = time.Now().UnixMilli()
		sm.mu.Unlock()

		// Always emit root subagent span on exit (uses traceCtx which is never cancelled).
		sm.emitSubagentSpan(traceCtx, subRootSpanID, taskStart, task, model, finalContent)
		slog.Debug("subagent tracing: root span emitted",
			"id", task.ID, "span_id", subRootSpanID,
			"trace_id", tracing.TraceIDFromContext(traceCtx),
			"status", task.Status, "iterations", iteration)

		// Schedule auto-archive
		if sm.config.ArchiveAfterMinutes > 0 {
			go sm.scheduleArchive(task.ID, time.Duration(sm.config.ArchiveAfterMinutes)*time.Minute)
		}
	}()

	if ctx.Err() != nil {
		sm.mu.Lock()
		task.Status = TaskStatusCancelled
		task.Result = "cancelled before execution"
		sm.mu.Unlock()
		return 0
	}

	// Build tools for subagent (no spawn/subagent tools to prevent recursion)
	toolsReg := sm.createTools()
	sm.applyDenyList(toolsReg, task.Depth)

	// Determine model (cascading priority matching TS sessions-spawn-tool.ts):
	// 1. Per-task model override (highest)
	// 2. SubagentConfig.Model (global subagent override)
	// 3. SubagentManager default model (inherited from parent)
	model = sm.model
	if sm.config.Model != "" {
		model = sm.config.Model
	}
	if task.Model != "" {
		model = task.Model
	}

	// Build subagent system prompt (matching TS buildSubagentSystemPrompt pattern).
	systemPrompt := sm.buildSubagentSystemPrompt(task)

	messages := []providers.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: task.Task},
	}

	// Run LLM iteration loop (similar to agent loop but simplified)
	maxIterations := 20

	for iteration < maxIterations {
		iteration++

		if ctx.Err() != nil {
			sm.mu.Lock()
			task.Status = TaskStatusCancelled
			task.Result = "cancelled during execution"
			sm.mu.Unlock()
			return iteration
		}

		chatReq := providers.ChatRequest{
			Messages: messages,
			Tools:    toolsReg.ProviderDefs(),
			Model:    model,
			Options: map[string]interface{}{
				"max_tokens":  4096,
				"temperature": 0.5,
			},
		}

		llmStart := time.Now().UTC()
		resp, err := sm.provider.Chat(ctx, chatReq)
		sm.emitLLMSpan(subTraceCtx, llmStart, iteration, model, messages, resp, err)

		if err != nil {
			sm.mu.Lock()
			task.Status = TaskStatusFailed
			task.Result = fmt.Sprintf("LLM error at iteration %d: %v", iteration, err)
			sm.mu.Unlock()
			slog.Warn("subagent LLM error", "id", task.ID, "iteration", iteration, "error", err)
			return iteration
		}

		// No tool calls → done
		if len(resp.ToolCalls) == 0 {
			finalContent = resp.Content
			break
		}

		// Build assistant message
		assistantMsg := providers.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		// Execute tools
		for _, tc := range resp.ToolCalls {
			slog.Debug("subagent tool call", "id", task.ID, "tool", tc.Name)

			toolStart := time.Now().UTC()
			result := toolsReg.Execute(ctx, tc.Name, tc.Arguments)

			argsJSON, _ := json.Marshal(tc.Arguments)
			sm.emitToolSpan(subTraceCtx, toolStart, tc.Name, tc.ID, string(argsJSON), result.ForLLM, result.IsError)

			messages = append(messages, providers.Message{
				Role:       "tool",
				Content:    result.ForLLM,
				ToolCallID: tc.ID,
			})
		}
	}

	sm.mu.Lock()
	if finalContent == "" {
		finalContent = "Task completed but no final response was generated."
	}
	task.Status = TaskStatusCompleted
	task.Result = finalContent
	sm.mu.Unlock()

	slog.Info("subagent completed", "id", task.ID, "iterations", iteration)
	return iteration
}

// applyDenyList removes denied tools from the registry based on depth.
func (sm *SubagentManager) applyDenyList(reg *Registry, depth int) {
	// Always deny
	for _, name := range SubagentDenyAlways {
		reg.Unregister(name)
	}

	// Leaf deny (at max depth)
	if depth >= sm.config.MaxSpawnDepth {
		for _, name := range SubagentDenyLeaf {
			reg.Unregister(name)
		}
	}
}

// buildSubagentSystemPrompt constructs the system prompt for a subagent,
// matching the TS buildSubagentSystemPrompt pattern from subagent-announce.ts.
func (sm *SubagentManager) buildSubagentSystemPrompt(task *SubagentTask) string {
	parentLabel := "main agent"
	if task.Depth >= 2 {
		parentLabel = "parent orchestrator"
	}

	canSpawn := task.Depth < sm.config.MaxSpawnDepth

	prompt := fmt.Sprintf(`# Subagent Context

You are a **subagent** spawned by the %s for a specific task.

## Your Role
- You were created to handle: %s
- Complete this task. That is your entire purpose.
- You are NOT the %s. Do not try to be.

## Rules
1. **Stay focused** — Do your assigned task, nothing else.
2. **Complete the task** — Your final message will be automatically reported to the %s.
3. **Never ask for clarification** — Work with what you have. If asked to create content, generate it yourself.
4. **Be ephemeral** — You may be terminated after task completion. That is fine.

## Output Format
Your final response IS the deliverable — it will be forwarded to the user.
- If asked to create content (posts, articles, messages, etc.), output the FULL content directly. Do NOT describe what you wrote — just write it.
- Do NOT say "I wrote a post about..." or "Here is what I created...". Output the content itself as your response.
- If the task is research or analysis, provide the complete findings.
- The %s will receive your exact final response, so make it user-ready.

## What You Do NOT Do
- NO user conversations (that is the %s's job)
- NO external messages unless explicitly tasked
- NO cron jobs or persistent state
- NO pretending to be the %s`,
		parentLabel, task.Task,
		parentLabel, parentLabel, parentLabel, parentLabel, parentLabel)

	if canSpawn {
		prompt += `

## Sub-Agent Spawning
You CAN spawn your own sub-agents for parallel or complex work using the spawn tool.
Your sub-agents will announce their results back to you automatically (not to the main agent).
Coordinate their work and synthesize results before reporting back.`
	} else if task.Depth >= 2 {
		prompt += `

## Sub-Agent Spawning
You are a leaf worker and CANNOT spawn further sub-agents. Focus on your assigned task.`
	}

	prompt += fmt.Sprintf(`

## Session Context
- Label: %s
- Depth: %d / %d`, task.Label, task.Depth, sm.config.MaxSpawnDepth)

	return prompt
}
