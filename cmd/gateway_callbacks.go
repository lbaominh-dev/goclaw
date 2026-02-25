package cmd

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/bootstrap"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// buildEnsureUserFiles creates the per-user file seeding callback.
// Used by both managed and standalone modes — no mode-specific logic.
// Seeds per-user context files on first chat (new user profile).
func buildEnsureUserFiles(as store.AgentStore) agent.EnsureUserFilesFunc {
	return func(ctx context.Context, agentID uuid.UUID, userID, agentType, workspace string) error {
		isNew, err := as.GetOrCreateUserProfile(ctx, agentID, userID, workspace)
		if err != nil {
			return err
		}
		if !isNew {
			return nil // already profiled = already seeded
		}

		// Auto-add first group member as a file writer (bootstrap the allowlist).
		if strings.HasPrefix(userID, "group:") {
			senderID := store.SenderIDFromContext(ctx)
			if senderID != "" {
				parts := strings.SplitN(senderID, "|", 2)
				numericID := parts[0]
				senderUsername := ""
				if len(parts) > 1 {
					senderUsername = parts[1]
				}
				if addErr := as.AddGroupFileWriter(ctx, agentID, userID, numericID, "", senderUsername); addErr != nil {
					slog.Warn("failed to auto-add group file writer", "error", addErr, "sender", numericID, "group", userID)
				}
			}
		}

		_, err = bootstrap.SeedUserFiles(ctx, as, agentID, userID, agentType)
		return err
	}
}

// buildBootstrapCleanup creates a callback that removes BOOTSTRAP.md for a user.
// Used as a safety net after enough conversation turns, in case the LLM
// didn't clear BOOTSTRAP.md itself. Idempotent — no-op if already cleared.
func buildBootstrapCleanup(as store.AgentStore) agent.BootstrapCleanupFunc {
	return func(ctx context.Context, agentID uuid.UUID, userID string) error {
		return as.DeleteUserContextFile(ctx, agentID, userID, bootstrap.BootstrapFile)
	}
}

// buildContextFileLoader creates the per-request context file loader callback.
// Used by both managed and standalone modes — no mode-specific logic.
// Delegates to the ContextFileInterceptor for type-aware routing.
func buildContextFileLoader(intc *tools.ContextFileInterceptor) agent.ContextFileLoaderFunc {
	return func(ctx context.Context, agentID uuid.UUID, userID, agentType string) []bootstrap.ContextFile {
		return intc.LoadContextFiles(ctx, agentID, userID, agentType)
	}
}
