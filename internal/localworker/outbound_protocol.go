package localworker

import "encoding/json"

const (
	OutboundEnvelopeJobDispatch = "job.dispatch"
	DefaultOutboundAuthHeader   = "Authorization"
)

// OutboundEnvelope is the minimal message wrapper sent to outbound workers.
type OutboundEnvelope struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

// OutboundJobDispatch is the payload shape for job.dispatch messages.
type OutboundJobDispatch struct {
	JobID       string          `json:"jobId"`
	RuntimeKind string          `json:"runtimeKind,omitempty"`
	Job         json.RawMessage `json:"job,omitempty"`
}

// WorkerReplyEnvelope is a minimal shape for future worker replies.
type WorkerReplyEnvelope struct {
	Type    string          `json:"type"`
	JobID   string          `json:"jobId,omitempty"`
	Status  string          `json:"status,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}
