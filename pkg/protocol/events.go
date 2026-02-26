package protocol

// WebSocket event names pushed from server to client.
const (
	EventAgent              = "agent"
	EventChat               = "chat"
	EventHealth             = "health"
	EventCron               = "cron"
	EventExecApprovalReq    = "exec.approval.requested"
	EventExecApprovalRes    = "exec.approval.resolved"
	EventPresence           = "presence"
	EventTick               = "tick"
	EventShutdown           = "shutdown"
	EventNodePairRequested  = "node.pair.requested"
	EventNodePairResolved   = "node.pair.resolved"
	EventDevicePairReq      = "device.pair.requested"
	EventDevicePairRes      = "device.pair.resolved"
	EventVoicewakeChanged   = "voicewake.changed"
	EventConnectChallenge   = "connect.challenge"
	EventHeartbeat          = "heartbeat"
	EventTalkMode           = "talk.mode"

	// Agent summoning events (predefined agent setup via LLM).
	EventAgentSummoning = "agent.summoning"

	// Agent handoff event (payload: from_agent, to_agent, reason).
	EventHandoff = "handoff"

	// Cache invalidation events (internal, not forwarded to WS clients).
	EventCacheInvalidate = "cache.invalidate"
)

// Agent event subtypes (in payload.type)
const (
	AgentEventRunStarted   = "run.started"
	AgentEventRunCompleted = "run.completed"
	AgentEventRunFailed    = "run.failed"
	AgentEventToolCall     = "tool.call"
	AgentEventToolResult   = "tool.result"
)

// Chat event subtypes (in payload.type)
const (
	ChatEventChunk     = "chunk"
	ChatEventMessage   = "message"
	ChatEventThinking  = "thinking"
)
