package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/scheduler"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

type postTurnTeamStoreStub struct {
	team           *store.TeamData
	task           *store.TeamTaskData
	completeCalled bool
	failCalled     bool
	renewCalled    bool
}

func (s *postTurnTeamStoreStub) CreateTeam(context.Context, *store.TeamData) error { return nil }
func (s *postTurnTeamStoreStub) GetTeam(_ context.Context, teamID uuid.UUID) (*store.TeamData, error) {
	if s.team != nil && s.team.ID == teamID {
		cp := *s.team
		return &cp, nil
	}
	return nil, nil
}
func (s *postTurnTeamStoreStub) GetTeamUnscoped(context.Context, uuid.UUID) (*store.TeamData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) UpdateTeam(context.Context, uuid.UUID, map[string]any) error {
	return nil
}
func (s *postTurnTeamStoreStub) DeleteTeam(context.Context, uuid.UUID) error         { return nil }
func (s *postTurnTeamStoreStub) ListTeams(context.Context) ([]store.TeamData, error) { return nil, nil }
func (s *postTurnTeamStoreStub) AddMember(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (s *postTurnTeamStoreStub) RemoveMember(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *postTurnTeamStoreStub) ListMembers(context.Context, uuid.UUID) ([]store.TeamMemberData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) ListIdleMembers(context.Context, uuid.UUID) ([]store.TeamMemberData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) GetTeamForAgent(context.Context, uuid.UUID) (*store.TeamData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) KnownUserIDs(context.Context, uuid.UUID, int) ([]string, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) ListTaskScopes(context.Context, uuid.UUID) ([]store.ScopeEntry, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) CreateTask(context.Context, *store.TeamTaskData) error { return nil }
func (s *postTurnTeamStoreStub) UpdateTask(context.Context, uuid.UUID, map[string]any) error {
	return nil
}
func (s *postTurnTeamStoreStub) ListTasks(context.Context, uuid.UUID, string, string, string, string, string, int, int) ([]store.TeamTaskData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) GetTask(_ context.Context, taskID uuid.UUID) (*store.TeamTaskData, error) {
	if s.task != nil && s.task.ID == taskID {
		cp := *s.task
		return &cp, nil
	}
	return nil, nil
}
func (s *postTurnTeamStoreStub) GetTasksByIDs(context.Context, []uuid.UUID) ([]store.TeamTaskData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) SearchTasks(context.Context, uuid.UUID, string, int, string) ([]store.TeamTaskData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) DeleteTask(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s *postTurnTeamStoreStub) DeleteTasks(context.Context, []uuid.UUID, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) ClaimTask(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *postTurnTeamStoreStub) AssignTask(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *postTurnTeamStoreStub) CompleteTask(context.Context, uuid.UUID, uuid.UUID, string) error {
	s.completeCalled = true
	return nil
}
func (s *postTurnTeamStoreStub) CancelTask(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (s *postTurnTeamStoreStub) FailTask(context.Context, uuid.UUID, uuid.UUID, string) error {
	s.failCalled = true
	return nil
}
func (s *postTurnTeamStoreStub) FailPendingTask(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (s *postTurnTeamStoreStub) ReviewTask(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s *postTurnTeamStoreStub) ApproveTask(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (s *postTurnTeamStoreStub) RejectTask(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (s *postTurnTeamStoreStub) UpdateTaskProgress(context.Context, uuid.UUID, uuid.UUID, int, string) error {
	return nil
}
func (s *postTurnTeamStoreStub) RenewTaskLock(context.Context, uuid.UUID, uuid.UUID) error {
	s.renewCalled = true
	return nil
}
func (s *postTurnTeamStoreStub) ResetTaskStatus(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *postTurnTeamStoreStub) ListActiveTasksByChatID(context.Context, string) ([]store.TeamTaskData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) AddTaskComment(context.Context, *store.TeamTaskCommentData) error {
	return nil
}
func (s *postTurnTeamStoreStub) ListTaskComments(context.Context, uuid.UUID) ([]store.TeamTaskCommentData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) ListRecentTaskComments(context.Context, uuid.UUID, int) ([]store.TeamTaskCommentData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) RecordTaskEvent(context.Context, *store.TeamTaskEventData) error {
	return nil
}
func (s *postTurnTeamStoreStub) ListTaskEvents(context.Context, uuid.UUID) ([]store.TeamTaskEventData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) ListTeamEvents(context.Context, uuid.UUID, int, int) ([]store.TeamTaskEventData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) AttachFileToTask(context.Context, *store.TeamTaskAttachmentData) error {
	return nil
}
func (s *postTurnTeamStoreStub) GetAttachment(context.Context, uuid.UUID) (*store.TeamTaskAttachmentData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) ListTaskAttachments(context.Context, uuid.UUID) ([]store.TeamTaskAttachmentData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) DetachFileFromTask(context.Context, uuid.UUID, string) error {
	return nil
}
func (s *postTurnTeamStoreStub) RecoverAllStaleTasks(context.Context) ([]store.RecoveredTaskInfo, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) ForceRecoverAllTasks(context.Context) ([]store.RecoveredTaskInfo, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) ListRecoverableTasks(context.Context, uuid.UUID) ([]store.TeamTaskData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) MarkAllStaleTasks(context.Context, time.Time) ([]store.RecoveredTaskInfo, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) MarkInReviewStaleTasks(context.Context, time.Time) ([]store.RecoveredTaskInfo, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) FixOrphanedBlockedTasks(context.Context) ([]store.RecoveredTaskInfo, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) SetTaskFollowup(context.Context, uuid.UUID, uuid.UUID, time.Time, int, string, string, string) error {
	return nil
}
func (s *postTurnTeamStoreStub) ClearTaskFollowup(context.Context, uuid.UUID) error { return nil }
func (s *postTurnTeamStoreStub) ListAllFollowupDueTasks(context.Context) ([]store.TeamTaskData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) IncrementFollowupCount(context.Context, uuid.UUID, *time.Time) error {
	return nil
}
func (s *postTurnTeamStoreStub) ClearFollowupByScope(context.Context, string, string) (int, error) {
	return 0, nil
}
func (s *postTurnTeamStoreStub) SetFollowupForActiveTasks(context.Context, uuid.UUID, string, string, time.Time, int, string) (int, error) {
	return 0, nil
}
func (s *postTurnTeamStoreStub) HasActiveMemberTasks(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}
func (s *postTurnTeamStoreStub) GrantTeamAccess(context.Context, uuid.UUID, string, string, string) error {
	return nil
}
func (s *postTurnTeamStoreStub) RevokeTeamAccess(context.Context, uuid.UUID, string) error {
	return nil
}
func (s *postTurnTeamStoreStub) ListTeamGrants(context.Context, uuid.UUID) ([]store.TeamUserGrant, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) ListUserTeams(context.Context, string) ([]store.TeamData, error) {
	return nil, nil
}
func (s *postTurnTeamStoreStub) HasTeamAccess(context.Context, uuid.UUID, string) (bool, error) {
	return false, nil
}

type postTurnStub struct{ dispatched bool }

func (p *postTurnStub) ProcessPendingTasks(context.Context, uuid.UUID, []uuid.UUID) error { return nil }
func (p *postTurnStub) DispatchUnblockedTasks(context.Context, uuid.UUID)                 { p.dispatched = true }

func TestResolveTeamTaskOutcome_QueuedRunSkipsAutoComplete(t *testing.T) {
	teamID := uuid.New()
	taskID := uuid.New()
	teamStore := &postTurnTeamStoreStub{
		team: &store.TeamData{BaseModel: store.BaseModel{ID: teamID}},
		task: &store.TeamTaskData{
			BaseModel:  store.BaseModel{ID: taskID},
			TeamID:     teamID,
			Status:     store.TeamTaskStatusInProgress,
			Subject:    "Implement local worker flow",
			TaskNumber: 12,
		},
	}
	postTurn := &postTurnStub{}
	deps := &ConsumerDeps{TeamStore: teamStore, PostTurn: postTurn}

	resolveTeamTaskOutcome(
		store.WithTenantID(context.Background(), store.MasterTenantID),
		deps,
		scheduler.RunOutcome{Result: &agent.RunResult{RunID: "run-1", Queued: true}},
		&tools.TaskActionFlags{},
		teammateTaskMeta{TaskID: taskID, TeamID: teamID, ToAgent: "member-1", Subject: "Implement local worker flow", TaskNumber: 12},
	)

	if teamStore.completeCalled {
		t.Fatal("queued local worker run should not auto-complete the task")
	}
	if teamStore.failCalled {
		t.Fatal("queued local worker run should not fail the task")
	}
	if !postTurn.dispatched {
		t.Fatal("queued local worker run should still dispatch unblocked tasks")
	}
}
