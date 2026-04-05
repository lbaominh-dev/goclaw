package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/localworker"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

type localWorkerOutput struct {
	Type  string
	Chunk string
}

func (l *Loop) dispatchLocalWorkerRun(ctx context.Context, req RunRequest) (*RunResult, error) {
	if l.localWorkerDispatcher == nil {
		return nil, fmt.Errorf("local worker dispatcher not configured")
	}
	if l.workerEndpointID == "" {
		return nil, fmt.Errorf("local worker endpoint binding is required")
	}
	rc := store.RunContextFromCtx(ctx)
	input := localworker.DispatchJobInput{
		TenantID:          store.TenantIDFromContext(ctx),
		WorkerEndpointID:  l.workerEndpointID,
		RuntimeKind:       l.localRuntimeKind,
		WorkspaceKey:      l.workspaceKey,
		AgentID:           l.agentUUID,
		AgentKey:          l.id,
		RunID:             req.RunID,
		SessionKey:        req.SessionKey,
		Message:           req.Message,
		UserID:            req.UserID,
		Channel:           req.Channel,
		ChannelType:       req.ChannelType,
		ChatID:            req.ChatID,
		PeerKind:          req.PeerKind,
		TeamID:            req.TeamID,
		TeamTaskID:        req.TeamTaskID,
		ParentAgentID:     req.ParentAgentID,
		LeaderAgentID:     req.LeaderAgentID,
		LocalKey:          req.LocalKey,
		WorkspaceChannel:  req.WorkspaceChannel,
		WorkspaceChatID:   req.WorkspaceChatID,
		TeamWorkspace:     req.TeamWorkspace,
		ExtraSystemPrompt: req.ExtraSystemPrompt,
		RunContext:        rc,
		TraceName:         req.TraceName,
		TraceTags:         append([]string(nil), req.TraceTags...),
		ModelOverride:     req.ModelOverride,
		LightContext:      req.LightContext,
		HideInput:         req.HideInput,
		ContentSuffix:     req.ContentSuffix,
		RunKind:           req.RunKind,
		ParentTraceID:     uuidString(req.ParentTraceID),
		ParentRootSpanID:  uuidString(req.ParentRootSpanID),
		LinkedTraceID:     uuidString(req.LinkedTraceID),
	}
	if taskID, err := parseOptionalUUID(req.TeamTaskID); err != nil {
		return nil, err
	} else {
		input.TaskID = taskID
	}
	job, err := l.localWorkerDispatcher.DispatchRun(ctx, input)
	if err != nil {
		return nil, err
	}
	if l.localWorkerWaiters == nil {
		return nil, fmt.Errorf("local worker waiter registry not configured")
	}
	emit := func(eventType string, payload any) {
		l.emit(AgentEvent{
			Type:       eventType,
			AgentID:    l.id,
			RunID:      req.RunID,
			Payload:    payload,
			UserID:     req.UserID,
			Channel:    req.Channel,
			ChatID:     req.ChatID,
			SessionKey: req.SessionKey,
			TenantID:   l.tenantID,
		})
	}
	emit(protocol.AgentEventActivity, map[string]any{
		"phase":    "queued_local_worker",
		"jobId":    job.ID.String(),
		"workerId": l.workerEndpointID,
	})

	updates := l.localWorkerWaiters.Subscribe(job.ID.String())
	defer l.localWorkerWaiters.Unsubscribe(job.ID.String(), updates)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case msg, ok := <-updates:
			if !ok {
				return nil, fmt.Errorf("local worker updates closed")
			}
			switch strings.TrimSpace(msg.Type) {
			case "job.started":
				emit(protocol.AgentEventActivity, map[string]any{
					"phase":    "running_local_worker",
					"jobId":    job.ID.String(),
					"workerId": l.workerEndpointID,
				})
			case "job.output", "job.status":
				output := parseLocalWorkerOutput(msg)
				if strings.TrimSpace(output.Chunk) != "" {
					switch output.Type {
					case "Final":
						emit(protocol.ChatEventChunk, map[string]any{"content": output.Chunk})
					default:
						emit(protocol.ChatEventThinking, map[string]any{"content": output.Chunk})
					}
				}
			case "job.completed":
				return &RunResult{
					RunID:      req.RunID,
					Content:    describeLocalWorkerReply(msg),
					Iterations: 0,
				}, nil
			case "job.failed":
				if isLocalWorkerCanceled(msg) {
					return nil, ctx.Err()
				}
				return nil, fmt.Errorf("%s", describeLocalWorkerReply(msg))
			}
		}
	}
}

func parseLocalWorkerOutput(reply localworker.WorkerReplyEnvelope) localWorkerOutput {
	var payload struct {
		Type    string `json:"type"`
		Chunk   string `json:"chunk"`
		Stream  string `json:"stream"`
		Text    string `json:"text"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(reply.Payload, &payload); err != nil {
		return localWorkerOutput{}
	}
	chunk := payload.Chunk
	if chunk == "" {
		chunk = payload.Text
	}
	if chunk == "" {
		chunk = payload.Message
	}
	kind := strings.TrimSpace(payload.Type)
	if kind == "" {
		kind = inferLocalWorkerOutputType(payload.Stream, chunk)
	}
	return localWorkerOutput{Type: kind, Chunk: chunk}
}

func inferLocalWorkerOutputType(stream, chunk string) string {
	switch {
	case strings.EqualFold(strings.TrimSpace(stream), "stderr"):
		return "Error"
	case strings.TrimSpace(chunk) == "":
		return ""
	default:
		return "Thinking"
	}
}

func describeLocalWorkerReply(reply localworker.WorkerReplyEnvelope) string {
	raw := reply.Payload
	if len(raw) == 0 {
		raw = reply.Error
	}
	if len(raw) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, key := range []string{"message", "delta", "text", "content", "line", "summary"} {
			if text, ok := obj[key].(string); ok {
				text = strings.TrimSpace(text)
				if text != "" {
					return text
				}
			}
		}
	}
	return strings.TrimSpace(string(raw))
}

func isLocalWorkerCanceled(reply localworker.WorkerReplyEnvelope) bool {
	var obj map[string]any
	if err := json.Unmarshal(reply.Error, &obj); err != nil {
		return false
	}
	if code, ok := obj["code"].(string); ok && strings.EqualFold(strings.TrimSpace(code), "CANCELED") {
		return true
	}
	if msg, ok := obj["message"].(string); ok && strings.Contains(strings.ToLower(msg), "canceled") {
		return true
	}
	return false
}

func parseOptionalUUID(raw string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse team task id: %w", err)
	}
	return &id, nil
}

func uuidString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}
