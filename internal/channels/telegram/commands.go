package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// resolveAgentUUID looks up the agent UUID from the channel's agent key.
// Returns uuid.Nil if the agent key is empty or not found.
func (c *Channel) resolveAgentUUID(ctx context.Context) (uuid.UUID, error) {
	key := c.AgentID()
	if key == "" {
		return uuid.Nil, fmt.Errorf("no agent key configured")
	}

	// Try direct UUID parse first (future-proofing).
	if id, err := uuid.Parse(key); err == nil {
		return id, nil
	}

	// Look up by agent key.
	agent, err := c.agentStore.GetByKey(ctx, key)
	if err != nil {
		return uuid.Nil, fmt.Errorf("agent %q not found: %w", key, err)
	}
	return agent.ID, nil
}

// handleBotCommand checks if the message is a known bot command and handles it.
// Returns true if the message was handled as a command.
func (c *Channel) handleBotCommand(ctx context.Context, message *telego.Message, chatID int64, chatIDStr, localKey, text, senderID string, isGroup, isForum bool, messageThreadID int) bool {
	if len(text) == 0 || text[0] != '/' {
		return false
	}

	// Extract command (strip @botname suffix if present)
	cmd := strings.SplitN(text, " ", 2)[0]
	cmd = strings.SplitN(cmd, "@", 2)[0]
	cmd = strings.ToLower(cmd)

	chatIDObj := tu.ID(chatID)

	// Helper: set MessageThreadID on outgoing messages for forum topics.
	// TS ref: buildTelegramThreadParams() — General topic (1) must be omitted.
	setThread := func(msg *telego.SendMessageParams) {
		sendThreadID := resolveThreadIDForSend(messageThreadID)
		if sendThreadID > 0 {
			msg.MessageThreadID = sendThreadID
		}
	}

	switch cmd {
	case "/start":
		// Don't intercept /start — let it pass through to agent loop.
		return false

	case "/help":
		helpText := "Available commands:\n" +
			"/start — Start chatting with the bot\n" +
			"/help — Show this help message\n" +
			"/stop — Stop current running task\n" +
			"/stopall — Stop all running tasks\n" +
			"/reset — Reset conversation history\n" +
			"/status — Show bot status\n" +
			"/tasks — List team tasks\n" +
			"/task_detail <id> — View task detail\n" +
			"/writers — List file writers for this group\n" +
			"/addwriter — Add a file writer (reply to their message)\n" +
			"/removewriter — Remove a file writer (reply to their message)\n" +
			"\nJust send a message to chat with the AI."
		msg := tu.Message(chatIDObj, helpText)
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
		return true

	case "/reset":
		// Fix: use correct PeerKind so the gateway consumer builds the right session key.
		peerKind := "direct"
		if isGroup {
			peerKind = "group"
		}
		c.Bus().PublishInbound(bus.InboundMessage{
			Channel:  c.Name(),
			SenderID: senderID,
			ChatID:   chatIDStr,
			Content:  "/reset",
			PeerKind: peerKind,
			AgentID:  c.AgentID(),
			UserID:   strings.SplitN(senderID, "|", 2)[0],
			Metadata: map[string]string{
				"command":           "reset",
				"local_key":         localKey,
				"is_forum":          fmt.Sprintf("%t", isForum),
				"message_thread_id": fmt.Sprintf("%d", messageThreadID),
			},
		})
		msg := tu.Message(chatIDObj, "Conversation history has been reset.")
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
		return true

	case "/stop":
		peerKind := "direct"
		if isGroup {
			peerKind = "group"
		}
		c.Bus().PublishInbound(bus.InboundMessage{
			Channel:  c.Name(),
			SenderID: senderID,
			ChatID:   chatIDStr,
			Content:  "/stop",
			PeerKind: peerKind,
			AgentID:  c.AgentID(),
			UserID:   strings.SplitN(senderID, "|", 2)[0],
			Metadata: map[string]string{
				"command":           "stop",
				"local_key":         localKey,
				"is_forum":          fmt.Sprintf("%t", isForum),
				"message_thread_id": fmt.Sprintf("%d", messageThreadID),
			},
		})
		// Feedback is sent by the consumer after cancel result is known.
		return true

	case "/stopall":
		peerKind := "direct"
		if isGroup {
			peerKind = "group"
		}
		c.Bus().PublishInbound(bus.InboundMessage{
			Channel:  c.Name(),
			SenderID: senderID,
			ChatID:   chatIDStr,
			Content:  "/stopall",
			PeerKind: peerKind,
			AgentID:  c.AgentID(),
			UserID:   strings.SplitN(senderID, "|", 2)[0],
			Metadata: map[string]string{
				"command":           "stopall",
				"local_key":         localKey,
				"is_forum":          fmt.Sprintf("%t", isForum),
				"message_thread_id": fmt.Sprintf("%d", messageThreadID),
			},
		})
		// Feedback is sent by the consumer after cancel result is known.
		return true

	case "/status":
		statusText := fmt.Sprintf("Bot status: Running\nChannel: Telegram\nBot: @%s", c.bot.Username())
		msg := tu.Message(chatIDObj, statusText)
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
		return true

	case "/tasks":
		c.handleTasksList(ctx, chatID, setThread)
		return true

	case "/task_detail":
		c.handleTaskDetail(ctx, chatID, text, setThread)
		return true

	case "/addwriter":
		c.handleWriterCommand(ctx, message, chatID, chatIDStr, senderID, isGroup, setThread, "add")
		return true

	case "/removewriter":
		c.handleWriterCommand(ctx, message, chatID, chatIDStr, senderID, isGroup, setThread, "remove")
		return true

	case "/writers":
		c.handleListWriters(ctx, chatID, chatIDStr, isGroup, setThread)
		return true
	}

	return false
}

// handleWriterCommand handles /addwriter and /removewriter commands.
// The target user is identified by replying to one of their messages.
func (c *Channel) handleWriterCommand(ctx context.Context, message *telego.Message, chatID int64, chatIDStr, senderID string, isGroup bool, setThread func(*telego.SendMessageParams), action string) {
	chatIDObj := tu.ID(chatID)

	send := func(text string) {
		msg := tu.Message(chatIDObj, text)
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
	}

	if !isGroup {
		send("This command only works in group chats.")
		return
	}

	if c.agentStore == nil {
		send("File writer management is not available.")
		return
	}

	agentID, err := c.resolveAgentUUID(ctx)
	if err != nil {
		slog.Debug("writer command: agent resolve failed", "error", err)
		send("File writer management is not available (no agent).")
		return
	}

	groupID := fmt.Sprintf("group:%s:%s", c.Name(), chatIDStr)
	senderNumericID := strings.SplitN(senderID, "|", 2)[0]

	// Check if sender is an existing writer (only writers can manage the list)
	isWriter, err := c.agentStore.IsGroupFileWriter(ctx, agentID, groupID, senderNumericID)
	if err != nil {
		slog.Warn("writer check failed", "error", err, "sender", senderNumericID)
		send("Failed to check permissions. Please try again.")
		return
	}
	if !isWriter {
		send("Only existing file writers can manage the writer list.")
		return
	}

	// Extract target user from reply-to message
	if message.ReplyToMessage == nil || message.ReplyToMessage.From == nil {
		verb := "add"
		if action == "remove" {
			verb = "remove"
		}
		send(fmt.Sprintf("To %s a writer: find a message from that person, swipe to reply it, then type /%swriter.", verb, verb))
		return
	}

	targetUser := message.ReplyToMessage.From
	targetID := fmt.Sprintf("%d", targetUser.ID)
	targetName := targetUser.FirstName
	if targetUser.Username != "" {
		targetName = "@" + targetUser.Username
	}

	switch action {
	case "add":
		if err := c.agentStore.AddGroupFileWriter(ctx, agentID, groupID, targetID, targetUser.FirstName, targetUser.Username); err != nil {
			slog.Warn("add writer failed", "error", err, "target", targetID)
			send("Failed to add writer. Please try again.")
			return
		}
		send(fmt.Sprintf("Added %s as a file writer.", targetName))

	case "remove":
		// Prevent removing the last writer
		writers, _ := c.agentStore.ListGroupFileWriters(ctx, agentID, groupID)
		if len(writers) <= 1 {
			send("Cannot remove the last file writer.")
			return
		}
		if err := c.agentStore.RemoveGroupFileWriter(ctx, agentID, groupID, targetID); err != nil {
			slog.Warn("remove writer failed", "error", err, "target", targetID)
			send("Failed to remove writer. Please try again.")
			return
		}
		send(fmt.Sprintf("Removed %s from file writers.", targetName))
	}
}

// handleListWriters handles the /writers command.
func (c *Channel) handleListWriters(ctx context.Context, chatID int64, chatIDStr string, isGroup bool, setThread func(*telego.SendMessageParams)) {
	chatIDObj := tu.ID(chatID)

	send := func(text string) {
		msg := tu.Message(chatIDObj, text)
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
	}

	if !isGroup {
		send("This command only works in group chats.")
		return
	}

	if c.agentStore == nil {
		send("File writer management is not available.")
		return
	}

	agentID, err := c.resolveAgentUUID(ctx)
	if err != nil {
		slog.Debug("list writers: agent resolve failed", "error", err)
		send("File writer management is not available (no agent).")
		return
	}

	groupID := fmt.Sprintf("group:%s:%s", c.Name(), chatIDStr)

	writers, err := c.agentStore.ListGroupFileWriters(ctx, agentID, groupID)
	if err != nil {
		slog.Warn("list writers failed", "error", err)
		send("Failed to list writers. Please try again.")
		return
	}

	if len(writers) == 0 {
		send("No file writers configured for this group. The first person to interact with the bot will be added automatically.")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File writers for this group (%d):\n", len(writers)))
	for i, w := range writers {
		label := w.UserID
		if w.Username != nil && *w.Username != "" {
			label = "@" + *w.Username
		} else if w.DisplayName != nil && *w.DisplayName != "" {
			label = *w.DisplayName
		}
		sb.WriteString(fmt.Sprintf("%d. %s (ID: %s)\n", i+1, label, w.UserID))
	}
	send(sb.String())
}

// --- Team tasks ---

const maxTasksInList = 30

// taskStatusIcon returns a short icon for each task status.
func taskStatusIcon(status string) string {
	switch status {
	case "completed":
		return "✅"
	case "in_progress":
		return "🔄"
	case "blocked":
		return "⛔"
	default: // pending
		return "⏳"
	}
}

// truncateStr truncates a string to maxLen runes, appending "…" if truncated.
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}

// handleTasksList handles the /tasks command — lists team tasks.
func (c *Channel) handleTasksList(ctx context.Context, chatID int64, setThread func(*telego.SendMessageParams)) {
	chatIDObj := tu.ID(chatID)

	send := func(text string) {
		msg := tu.Message(chatIDObj, text)
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
	}

	if c.teamStore == nil {
		send("Team features are not available.")
		return
	}

	agentID, err := c.resolveAgentUUID(ctx)
	if err != nil {
		slog.Debug("tasks command: agent resolve failed", "error", err)
		send("Team features are not available (no agent).")
		return
	}

	team, err := c.teamStore.GetTeamForAgent(ctx, agentID)
	if err != nil {
		slog.Warn("tasks command: GetTeamForAgent failed", "error", err)
		send("Failed to look up team. Please try again.")
		return
	}
	if team == nil {
		send("This agent is not part of any team.")
		return
	}

	tasks, err := c.teamStore.ListTasks(ctx, team.ID, "newest")
	if err != nil {
		slog.Warn("tasks command: ListTasks failed", "error", err)
		send("Failed to list tasks. Please try again.")
		return
	}

	if len(tasks) == 0 {
		send(fmt.Sprintf("No tasks for team %q.", team.Name))
		return
	}

	total := len(tasks)
	if total > maxTasksInList {
		tasks = tasks[:maxTasksInList]
	}

	var sb strings.Builder
	if total > maxTasksInList {
		sb.WriteString(fmt.Sprintf("Tasks for team %q (showing %d of %d):\n\n", team.Name, maxTasksInList, total))
	} else {
		sb.WriteString(fmt.Sprintf("Tasks for team %q (%d):\n\n", team.Name, total))
	}
	for i, t := range tasks {
		owner := ""
		if t.OwnerAgentKey != "" {
			owner = " — @" + t.OwnerAgentKey
		}
		sb.WriteString(fmt.Sprintf("%d. %s %s%s\n", i+1, taskStatusIcon(t.Status), t.Subject, owner))
	}
	sb.WriteString("\nTap a button below to view details.")

	// Build inline keyboard — one button per task.
	var rows [][]telego.InlineKeyboardButton
	for i, t := range tasks {
		label := fmt.Sprintf("%d. %s %s", i+1, taskStatusIcon(t.Status), truncateStr(t.Subject, 35))
		rows = append(rows, []telego.InlineKeyboardButton{
			{Text: label, CallbackData: "td:" + t.ID.String()},
		})
	}

	msg := tu.Message(chatIDObj, sb.String())
	setThread(msg)
	if len(rows) > 0 {
		msg.ReplyMarkup = &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	c.bot.SendMessage(ctx, msg)
}

// handleTaskDetail handles the /task_detail command — shows detail for a task.
func (c *Channel) handleTaskDetail(ctx context.Context, chatID int64, text string, setThread func(*telego.SendMessageParams)) {
	chatIDObj := tu.ID(chatID)

	send := func(t string) {
		for _, chunk := range chunkPlainText(t, telegramMaxMessageLen) {
			msg := tu.Message(chatIDObj, chunk)
			setThread(msg)
			c.bot.SendMessage(ctx, msg)
		}
	}

	// Extract task ID argument: "/task_detail <id>" or "/task_detail@botname <id>"
	parts := strings.SplitN(text, " ", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		send("Usage: /task_detail <task_id>")
		return
	}
	taskIDArg := strings.TrimSpace(parts[1])

	if c.teamStore == nil {
		send("Team features are not available.")
		return
	}

	agentID, err := c.resolveAgentUUID(ctx)
	if err != nil {
		slog.Debug("task_detail command: agent resolve failed", "error", err)
		send("Team features are not available (no agent).")
		return
	}

	team, err := c.teamStore.GetTeamForAgent(ctx, agentID)
	if err != nil {
		slog.Warn("task_detail command: GetTeamForAgent failed", "error", err)
		send("Failed to look up team. Please try again.")
		return
	}
	if team == nil {
		send("This agent is not part of any team.")
		return
	}

	tasks, err := c.teamStore.ListTasks(ctx, team.ID, "newest")
	if err != nil {
		slog.Warn("task_detail command: ListTasks failed", "error", err)
		send("Failed to list tasks. Please try again.")
		return
	}

	// Find task by full UUID or prefix match.
	taskIDLower := strings.ToLower(taskIDArg)
	for i := range tasks {
		tid := tasks[i].ID.String()
		if tid == taskIDLower || strings.HasPrefix(tid, taskIDLower) {
			send(formatTaskDetail(&tasks[i]))
			return
		}
	}

	send(fmt.Sprintf("Task %q not found. Use /tasks to see available tasks.", taskIDArg))
}

// handleCallbackQuery handles inline keyboard button presses.
func (c *Channel) handleCallbackQuery(ctx context.Context, query *telego.CallbackQuery) {
	// Always answer to dismiss the loading indicator.
	c.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
	})

	if !strings.HasPrefix(query.Data, "td:") {
		return
	}

	taskIDStr := strings.TrimPrefix(query.Data, "td:")

	// Resolve chat ID from the callback's message.
	chatID := query.Message.GetChat().ID
	chatIDObj := tu.ID(chatID)

	send := func(text string) {
		for _, chunk := range chunkPlainText(text, telegramMaxMessageLen) {
			msg := tu.Message(chatIDObj, chunk)
			c.bot.SendMessage(ctx, msg)
		}
	}

	if c.teamStore == nil {
		send("Team features are not available.")
		return
	}

	agentID, err := c.resolveAgentUUID(ctx)
	if err != nil {
		send("Team features are not available (no agent).")
		return
	}

	team, err := c.teamStore.GetTeamForAgent(ctx, agentID)
	if err != nil || team == nil {
		send("Could not resolve team.")
		return
	}

	tasks, err := c.teamStore.ListTasks(ctx, team.ID, "newest")
	if err != nil {
		send("Failed to list tasks.")
		return
	}

	for i := range tasks {
		if tasks[i].ID.String() == taskIDStr {
			send(formatTaskDetail(&tasks[i]))
			return
		}
	}
	send(fmt.Sprintf("Task %s not found.", taskIDStr[:8]))
}

// formatTaskDetail formats a single task for display.
func formatTaskDetail(t *store.TeamTaskData) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Task: %s\n", t.Subject))
	sb.WriteString(fmt.Sprintf("ID: %s\n", t.ID.String()))
	sb.WriteString(fmt.Sprintf("Status: %s %s\n", taskStatusIcon(t.Status), t.Status))
	if t.OwnerAgentKey != "" {
		sb.WriteString(fmt.Sprintf("Owner: @%s\n", t.OwnerAgentKey))
	}
	sb.WriteString(fmt.Sprintf("Priority: %d\n", t.Priority))
	if !t.CreatedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("Created: %s\n", t.CreatedAt.Format("2006-01-02 15:04")))
	}
	if t.Description != "" {
		sb.WriteString(fmt.Sprintf("\nDescription:\n%s\n", t.Description))
	}
	if t.Result != nil && *t.Result != "" {
		sb.WriteString(fmt.Sprintf("\nResult:\n%s\n", *t.Result))
	}
	if len(t.BlockedBy) > 0 {
		ids := make([]string, len(t.BlockedBy))
		for j, bid := range t.BlockedBy {
			ids[j] = bid.String()[:8]
		}
		sb.WriteString(fmt.Sprintf("\nBlocked by: %s\n", strings.Join(ids, ", ")))
	}
	return sb.String()
}

// --- Pairing UX ---

// buildPairingReply builds the pairing reply message matching TS behavior.
func buildPairingReply(telegramUserID, code string) string {
	return fmt.Sprintf(
		"GoClaw: access not configured.\n\nYour Telegram user id: %s\n\nPairing code: %s\n\nAsk the bot owner to approve with:\n  goclaw pairing approve %s",
		telegramUserID, code, code,
	)
}

// sendPairingReply generates a pairing code and sends the reply to the user.
// Debounces: won't send another reply to the same user within 60 seconds.
func (c *Channel) sendPairingReply(ctx context.Context, chatID int64, userID, username string) {
	if c.pairingService == nil {
		return
	}

	if lastSent, ok := c.pairingReplySent.Load(userID); ok {
		if time.Since(lastSent.(time.Time)) < pairingReplyDebounce {
			slog.Debug("pairing reply debounced", "user_id", userID)
			return
		}
	}

	code, err := c.pairingService.RequestPairing(userID, c.Name(), fmt.Sprintf("%d", chatID), "default")
	if err != nil {
		slog.Debug("pairing request failed", "user_id", userID, "error", err)
		return
	}

	replyText := buildPairingReply(userID, code)
	msg := tu.Message(tu.ID(chatID), replyText)
	if _, err := c.bot.SendMessage(ctx, msg); err != nil {
		slog.Warn("failed to send pairing reply", "chat_id", chatID, "error", err)
	} else {
		c.pairingReplySent.Store(userID, time.Now())
		slog.Info("telegram pairing reply sent",
			"user_id", userID, "username", username, "code", code,
		)
	}
}

// sendGroupPairingReply generates a pairing code for a group and sends the reply.
// Debounces: won't send another reply to the same group within 60 seconds.
func (c *Channel) sendGroupPairingReply(ctx context.Context, chatID int64, chatIDStr, groupSenderID string) {
	if lastSent, ok := c.pairingReplySent.Load(chatIDStr); ok {
		if time.Since(lastSent.(time.Time)) < pairingReplyDebounce {
			return
		}
	}

	code, err := c.pairingService.RequestPairing(groupSenderID, c.Name(), chatIDStr, "default")
	if err != nil {
		slog.Debug("group pairing request failed", "chat_id", chatIDStr, "error", err)
		return
	}

	replyText := fmt.Sprintf(
		"This group is not approved yet.\n\nPairing code: %s\n\nAsk the bot owner to approve with:\n  goclaw pairing approve %s",
		code, code,
	)
	msg := tu.Message(tu.ID(chatID), replyText)
	if _, err := c.bot.SendMessage(ctx, msg); err != nil {
		slog.Warn("failed to send group pairing reply", "chat_id", chatIDStr, "error", err)
	} else {
		c.pairingReplySent.Store(chatIDStr, time.Now())
		slog.Info("telegram group pairing reply sent", "chat_id", chatIDStr, "code", code)
	}
}

// SendPairingApproved sends the approval notification to a user.
func (c *Channel) SendPairingApproved(ctx context.Context, chatID, botName string) error {
	id, err := parseChatID(chatID)
	if err != nil {
		return fmt.Errorf("invalid chat ID: %w", err)
	}
	if botName == "" {
		botName = "GoClaw"
	}

	msg := tu.Message(tu.ID(id), fmt.Sprintf("✅ %s access approved. Send a message to start chatting.", botName))
	_, err = c.bot.SendMessage(ctx, msg)
	return err
}

// SyncMenuCommands registers bot commands with Telegram via setMyCommands.
func (c *Channel) SyncMenuCommands(ctx context.Context, commands []telego.BotCommand) error {
	if err := c.bot.DeleteMyCommands(ctx, nil); err != nil {
		slog.Debug("deleteMyCommands failed (may not exist)", "error", err)
	}

	if len(commands) == 0 {
		return nil
	}

	if len(commands) > 100 {
		commands = commands[:100]
	}

	return c.bot.SetMyCommands(ctx, &telego.SetMyCommandsParams{
		Commands: commands,
	})
}

// DefaultMenuCommands returns the default bot menu commands.
func DefaultMenuCommands() []telego.BotCommand {
	return []telego.BotCommand{
		{Command: "start", Description: "Start chatting with the bot"},
		{Command: "help", Description: "Show available commands"},
		{Command: "stop", Description: "Stop current running task"},
		{Command: "stopall", Description: "Stop all running tasks"},
		{Command: "reset", Description: "Reset conversation history"},
		{Command: "status", Description: "Show bot status"},
		{Command: "tasks", Description: "List team tasks"},
		{Command: "task_detail", Description: "View task detail by ID"},
		{Command: "writers", Description: "List file writers for this group"},
		{Command: "addwriter", Description: "Add a file writer (reply to their message)"},
		{Command: "removewriter", Description: "Remove a file writer (reply to their message)"},
	}
}
