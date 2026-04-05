# Worker Output Semantics And Session Reuse Design

## Goal

Improve local-worker chat behavior in two ways:

1. Worker `job.output` events should carry semantic output types such as `Thinking` instead of exposing raw transport payloads directly in the UI.
2. Worker-backed chat should reuse the same worker session for every message sent within the same web chat session.

The result should be cleaner streaming text in the chat UI and stable conversation continuity for local-worker agents.

## Current State

### Worker output transport

The opencode worker currently emits:

- `job.started`
- `job.output` with payload `{ stream: "stdout" | "stderr", chunk: string }`
- `job.completed`
- `job.failed`

The gateway and agent loop flatten these replies into text via `describeLocalWorkerReply(...)`. That makes the chat experience depend on the raw worker payload shape and provides no semantic classification beyond best-effort text rendering.

### Session behavior

Web chat uses a canonical `sessionKey` in the route and sends it through `chat.send`. When the page already has a session in the URL, the same key is reused for subsequent sends. When starting a new chat, the frontend creates a new session key before the first send.

The worker dispatch payload already includes the chat `sessionKey`, but worker-side output and prompt handling currently do not define a stronger contract for treating that key as the durable worker conversation identity.

## Requirements

### Functional requirements

1. `job.output` must support semantic labels for worker output.
2. The text shown in the chat UI must come from the output `chunk`, not from rendering the full payload object.
3. All sends in the same web chat session must reuse the same worker session identity.
4. Starting a new chat must start a new worker session identity.
5. Existing non-worker chat behavior must remain unchanged.

### Non-functional requirements

1. Preserve backward compatibility for the current worker transport during rollout.
2. Keep the visible UI change minimal: better streamed text, no raw payload dumps.
3. Avoid worker-runtime-specific UI logic in the frontend.
4. Keep the implementation small and localized to the worker, gateway, agent loop, and chat UI event handling.

## Recommended Approach

Normalize worker output at the gateway and agent boundary while preserving `chunk` as the text transport field.

This approach keeps the worker protocol lightweight, avoids pushing worker-specific rendering rules into the UI, and lets the backend provide a stable semantic contract for all local-worker runtimes.

## Design

### 1. Worker output contract

Extend `job.output` payload to support an optional semantic type while retaining the current `chunk` field.

Target shape:

```json
{
  "type": "job.output",
  "jobId": "...",
  "payload": {
    "type": "Thinking",
    "chunk": "Checking repository state...",
    "stream": "stdout"
  }
}
```

Rules:

1. `chunk` is the canonical text field for rendering.
2. `payload.type` is the semantic label.
3. `stream` remains optional compatibility metadata for worker diagnostics and fallback inference.
4. If `payload.type` is absent, the backend infers the semantic label from existing fields.

Initial semantic labels:

1. `Thinking`
2. `Action`
3. `Error`
4. `Final`

These labels are intentionally small and stable. They describe user-visible meaning, not transport internals.

### 2. Backend normalization

The Go local-worker path should stop treating the worker payload as display-ready output. Instead it should parse the reply into a normalized internal structure:

```go
type LocalWorkerOutput struct {
    Type  string
    Chunk string
}
```

Normalization rules:

1. If `payload.type` exists, use it.
2. If `payload.type` is missing:
   - `stderr` defaults to `Error` unless content better matches an action/progress line.
   - `stdout` defaults to `Thinking` during streaming.
3. `job.completed` may produce a final status text, but should not dump the whole payload to the chat stream.
4. `job.failed` should surface concise error text only.

The existing helper that turns worker replies into strings should be replaced or split into:

1. a parser that extracts semantic type plus `chunk`
2. a formatter that decides how each type maps into agent events and user-visible status text

### 3. Agent event mapping

The agent loop should map normalized worker output into existing chat event channels where possible.

Recommended mapping:

1. `Thinking` -> emit `thinking` events with `content=chunk`
2. `Final` -> emit `chunk` events with `content=chunk`
3. `Action` -> emit `activity` updates and optionally `thinking` text when the action text is useful to the user
4. `Error` -> emit concise stream text or failure text, but do not serialize raw payloads

This keeps the UI contract simple because the chat page already understands `thinking`, `chunk`, and `activity`.

### 4. UI rendering behavior

The web UI should continue rendering chat output from agent events, but it must rely on the normalized text content only.

Rules:

1. Never render raw worker payload objects in the chat transcript.
2. Continue using `thinking` for transient reasoning text.
3. Continue using `chunk` for assistant-visible output text.
4. If worker-specific semantic labels are later surfaced in the UI, they should be passed as metadata, not as direct payload dumps.

This keeps the UI decoupled from worker protocol details.

### 5. Session reuse behavior

The web chat session key is the source of truth for worker conversation continuity.

Rules:

1. If the user sends multiple messages in the same chat route, the exact same `sessionKey` must be forwarded to the worker every time.
2. The worker runtime must treat that `sessionKey` as the stable conversation identity.
3. When the user starts a new chat and a new `sessionKey` is created, the worker must treat it as a new conversation.

For the opencode worker, this means command construction and prompt/session handling must consistently use the forwarded `sessionKey` rather than generating a per-job conversation identity.

### 6. Compatibility and rollout

The rollout should be backward compatible.

Rules:

1. Existing workers that only send `{ stream, chunk }` continue to work.
2. The backend infers semantic labels when `payload.type` is absent.
3. Updated workers may begin sending `payload.type` immediately.
4. The UI does not need to branch on worker version.

## Data Flow

### Same-session message flow

1. User stays on `/chat/:sessionKey`
2. Frontend calls `chat.send` with that exact `sessionKey`
3. Gateway resolves the local-worker agent
4. Worker dispatch payload includes the same `sessionKey`
5. Worker uses that key as the durable conversation/session identity
6. Worker emits typed `job.output` chunks
7. Backend normalizes them and emits `thinking`, `chunk`, and `activity` events
8. UI renders only normalized text content

### New-chat flow

1. Frontend creates a new `sessionKey`
2. Route changes to the new chat session
3. First send forwards that new `sessionKey`
4. Worker treats it as a new conversation identity

## Error Handling

1. If worker output payload is malformed, the backend should ignore the malformed fields and avoid rendering raw JSON.
2. If `chunk` is empty, no user-visible text should be appended.
3. If semantic type is unknown, default to `Thinking` for non-error streaming text.
4. `job.failed` should emit a concise failure message and end the run.

## Testing

### Go tests

1. Worker reply normalization with explicit `payload.type`
2. Worker reply normalization fallback from `{ stream, chunk }`
3. `job.output` rendering uses only `chunk`
4. Repeated sends in the same session preserve the same dispatched `sessionKey`

### Worker tests

1. Opencode runner emits `chunk` consistently
2. Opencode runner includes semantic type when available
3. Same-session jobs reuse the same worker conversation identity derived from `sessionKey`

### UI tests

1. Streamed chat renders normalized text only
2. No raw payload object appears in the transcript
3. Same route session continues one worker conversation

## Out Of Scope

1. Redesigning the full chat event model
2. Exposing a rich worker-debug console in the main chat UI
3. Adding per-runtime semantic taxonomies beyond the initial shared set

## Implementation Notes

Prefer the smallest viable change set:

1. Extend worker payload shape without breaking current transport
2. Normalize in Go before UI delivery
3. Keep UI rendering based on existing `thinking` and `chunk` behavior
4. Ensure worker session identity is derived from the existing chat `sessionKey`

## Success Criteria

1. Local-worker chat no longer shows raw payload-shaped output in the transcript.
2. Worker output is semantically classified with stable labels.
3. Messages sent in one UI chat session continue the same worker conversation.
4. Starting a new UI chat starts a new worker conversation.
