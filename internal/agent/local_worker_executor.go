package agent

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/localworker"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

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
	l.emit(AgentEvent{
		Type:    protocol.AgentEventActivity,
		AgentID: l.id,
		RunID:   req.RunID,
		Payload: map[string]any{
			"phase":    "queued_local_worker",
			"jobId":    job.ID.String(),
			"workerId": l.workerEndpointID,
		},
	})
	return &RunResult{
		RunID:      req.RunID,
		Content:    "",
		Iterations: 0,
		Queued:     true,
	}, nil
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
