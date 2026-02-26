# 00 - Architecture Overview

## 1. Overview

GoClaw is an AI agent gateway written in Go. It exposes a WebSocket RPC (v3) interface and an OpenAI-compatible HTTP API for orchestrating LLM-powered agents. The system supports two operating modes:

- **Standalone** -- file-based storage, single-user, zero external dependencies beyond an LLM API key.
- **Managed** -- PostgreSQL-backed multi-tenant mode with HTTP CRUD APIs, per-user context files, encrypted credentials, and LLM call tracing.

> **Documentation scope**: This documentation is written for **managed mode** (PostgreSQL multi-tenant), which is the recommended production deployment. Standalone mode is functionally equivalent to the original OpenClaw -- all users share the same workspace, sessions, and context files with no per-user isolation, no encrypted secrets, no tracing, and no HTTP CRUD APIs. The remainder of this documentation assumes managed mode unless stated otherwise.

## 2. Component Diagram

```mermaid
flowchart TD
    subgraph Clients
        WS[WebSocket Clients]
        HTTP[HTTP Clients]
        TG[Telegram]
        DC[Discord]
        FS[Feishu / Lark]
        ZL[Zalo]
        WA[WhatsApp]
    end

    subgraph Gateway["Gateway Server"]
        WSS[WebSocket Server]
        HTTPS[HTTP API Server]
        MR[Method Router]
        RL[Rate Limiter]
        RBAC[Permission Engine]
    end

    subgraph Channels["Channel Manager"]
        CM[Channel Manager]
        PA[Pairing Service]
    end

    subgraph Core["Core Engine"]
        BUS[Message Bus]
        SCHED[Scheduler -- 3 Lanes]
        AR[Agent Router]
        LOOP[Agent Loop -- Think / Act / Observe]
    end

    subgraph Providers["LLM Providers"]
        ANTH[Anthropic -- Native HTTP + SSE]
        OAI[OpenAI-Compatible -- HTTP + SSE]
    end

    subgraph Tools["Tool Registry"]
        FS_T[Filesystem]
        EXEC[Exec / Shell]
        WEB[Web Search / Fetch]
        MEM[Memory]
        SUB[Subagent]
        TTS_T[TTS]
        BROW[Browser]
        SK[Skills]
        MCP_T[MCP Bridge]
        CT[Custom Tools]
    end

    subgraph Store["Store Layer"]
        SESS[SessionStore]
        AGENT_S[AgentStore]
        PROV_S[ProviderStore]
        CRON_S[CronStore]
        MEM_S[MemoryStore]
        SKILL_S[SkillStore]
        TRACE_S[TracingStore]
        MCP_S[MCPServerStore]
        CT_S[CustomToolStore]
    end

    WS --> WSS
    HTTP --> HTTPS
    TG & DC & FS & ZL & WA --> CM

    WSS --> MR
    HTTPS --> MR
    MR --> RL --> RBAC --> AR

    CM --> BUS
    BUS --> SCHED
    SCHED --> AR
    AR --> LOOP

    LOOP --> Providers
    LOOP --> Tools
    Tools --> Store
    LOOP --> Store
```

## 3. Module Map

| Module | Description |
|--------|-------------|
| `internal/gateway/` | WebSocket + HTTP server, client handling, method router |
| `internal/gateway/methods/` | RPC method handlers: chat, agents, sessions, config, skills, cron, pairing, exec approval, usage, send |
| `internal/agent/` | Agent loop (think, act, observe), router, resolver, system prompt builder, sanitization, pruning, tracing, memory flush |
| `internal/providers/` | LLM providers: Anthropic (native HTTP + SSE streaming), OpenAI-compatible (HTTP + SSE), retry logic |
| `internal/tools/` | Tool registry, filesystem ops, exec/shell, policy engine, subagent, context file + memory interceptors, credential scrubbing, rate limiting |
| `internal/tools/dynamic_loader.go` | Custom tool loader: LoadGlobal (startup), LoadForAgent (per-agent clone), ReloadGlobal (cache invalidation) |
| `internal/tools/dynamic_tool.go` | Custom tool executor: command template rendering, shell escaping, encrypted env vars |
| `internal/store/` | Store interfaces: SessionStore, AgentStore, ProviderStore, SkillStore, MemoryStore, CronStore, PairingStore, TracingStore, MCPServerStore |
| `internal/store/pg/` | PostgreSQL implementations (`database/sql` + `pgx/v5`) |
| `internal/store/file/` | File-based implementations (standalone mode) |
| `internal/bootstrap/` | System prompt files (AGENTS.md, SOUL.md, TOOLS.md, IDENTITY.md, USER.md, HEARTBEAT.md, BOOTSTRAP.md) + seeding + truncation |
| `internal/config/` | Config loading (JSON5) + env var overlay |
| `internal/skills/` | SKILL.md loader (5-tier hierarchy: workspace → project → personal → global → builtin) + BM25 search + hot-reload via fsnotify |
| `internal/channels/` | Channel manager + adapters: Telegram, Feishu/Lark, Zalo, Discord, WhatsApp |
| `internal/mcp/` | MCP server bridge (stdio, SSE, streamable-HTTP transports) |
| `internal/scheduler/` | Lane-based concurrency control (main, subagent, cron lanes) with per-session serialization |
| `internal/memory/` | Memory system (SQLite FTS5 + embeddings for standalone mode) |
| `internal/permissions/` | RBAC policy engine (admin, operator, viewer roles) |
| `internal/pairing/` | DM/device pairing service (8-character codes) |
| `internal/sessions/` | File-based session manager (standalone mode) |
| `internal/bus/` | Event pub/sub (Message Bus) |
| `internal/sandbox/` | Docker-based code execution sandbox |
| `internal/tts/` | Text-to-Speech providers: OpenAI, ElevenLabs, Edge, MiniMax |
| `internal/http/` | HTTP API handlers: /v1/chat/completions, /v1/agents, /v1/skills, /v1/traces, /v1/mcp |
| `internal/crypto/` | AES-256-GCM encryption for API keys |
| `internal/tracing/` | LLM call tracing (traces + spans), in-memory buffer with periodic store flush |
| `internal/tracing/otelexport/` | Optional OpenTelemetry OTLP exporter (opt-in via build tags; adds gRPC + protobuf) |
| `internal/heartbeat/` | Periodic agent wake-up service |

---

## 4. Two Operating Modes

| Aspect | Standalone | Managed |
|--------|-----------|---------|
| Config source | `config.json` + env vars | `config.json` + `GOCLAW_POSTGRES_DSN` |
| Storage | JSON files on disk | PostgreSQL |
| Agents | Defined in `config.json` `agents.list`, created eagerly at startup | `agents` table, lazy-resolved via `ManagedResolver` |
| Context files | Workspace filesystem (SOUL.md, IDENTITY.md, etc.) | `agent_context_files` + `user_context_files` tables |
| Agent types | N/A | `open` (7 per-user files) / `predefined` (agent-level + USER.md per-user) |
| Skills | Filesystem only (workspace + global dirs) | PostgreSQL + filesystem + embedding search |
| Memory | SQLite FTS5 + embeddings | pgvector hybrid (full-text search + vector similarity) |
| Tracing | N/A | `traces` + `spans` tables + optional OTel OTLP export |
| MCP servers | `config.json` `tools.mcp_servers` | `mcp_servers` table + grants |
| API key storage | `.env.local` / env vars only | PostgreSQL (AES-256-GCM encrypted) |
| HTTP CRUD API | N/A | `/v1/agents`, `/v1/skills`, `/v1/traces`, `/v1/mcp` |
| Virtual FS | Direct disk I/O | `ContextFileInterceptor` routes read_file/write_file to database |
| Custom tools | N/A | `custom_tools` table + `DynamicToolLoader` |
| Managed-only stores (nil in standalone) | -- | AgentStore, ProviderStore, TracingStore, MCPServerStore, CustomToolStore |

---

## 5. Multi-Tenant Identity Model

GoClaw uses the **Identity Propagation** pattern (also known as **Trusted Subsystem**). It does not implement authentication or authorization — instead, it trusts the upstream service that authenticates with the gateway token to provide accurate user identity.

```mermaid
flowchart LR
    subgraph "Upstream Service (trusted)"
        AUTH["Authenticate end-user"]
        HDR["Set X-GoClaw-User-Id header<br/>or user_id in WS connect"]
    end

    subgraph "GoClaw Gateway"
        EXTRACT["Extract user_id<br/>(opaque, VARCHAR 255)"]
        CTX["store.WithUserID(ctx)"]
        SCOPE["Per-user scoping:<br/>sessions, context files,<br/>memory, traces, agent shares"]
    end

    AUTH --> HDR
    HDR --> EXTRACT
    EXTRACT --> CTX
    CTX --> SCOPE
```

### Identity Flow

| Entry Point | How user_id is provided | Enforcement |
|-------------|------------------------|-------------|
| HTTP API | `X-GoClaw-User-Id` header | Required in managed mode |
| WebSocket | `user_id` field in `connect` handshake | Required in managed mode |
| Channels | Derived from platform sender ID (e.g., Telegram user ID) | Automatic |

### Compound User ID Convention

The `user_id` field is **opaque** to GoClaw — it does not interpret or validate the format. For multi-tenant deployments, the recommended convention is:

```
tenant.{tenantId}.user.{userId}
```

This hierarchical format ensures natural isolation between tenants. Since `user_id` is used as a scoping key across all per-user tables (`user_context_files`, `user_agent_profiles`, `user_agent_overrides`, `agent_shares`, `sessions`, `traces`), the compound format guarantees that users from different tenants cannot access each other's data.

### Where user_id is used

| Component | Usage |
|-----------|-------|
| Session keys | `agent:{agentId}:{channel}:direct:{peerId}` — peerId derived from user_id |
| Context files | `user_context_files` table scoped by `(agent_id, user_id)` |
| User profiles | `user_agent_profiles` table — first/last seen, workspace |
| User overrides | `user_agent_overrides` — per-user provider/model preferences |
| Agent shares | `agent_shares` table — user-level access control |
| Memory | Per-user memory entries via context propagation |
| Traces | `traces` table includes `user_id` for filtering |
| MCP grants | `mcp_user_grants` — per-user MCP server access |
| Skills grants | `skill_user_grants` — per-user skill access |

---

## 6. Gateway Startup Sequence

```mermaid
sequenceDiagram
    participant CLI as CLI (cmd/root.go)
    participant GW as runGateway()
    participant PG as PostgreSQL
    participant Engine as Core Engine

    CLI->>GW: 1. Parse CLI flags + load config
    GW->>GW: 2. Resolve workspace + data dirs
    GW->>GW: 3. Create Message Bus

    alt Managed mode
        GW->>PG: 4. Connect to Postgres (pg.NewPGStores)
        PG-->>GW: PG stores created
        GW->>GW: 5. Start tracing collector
        GW->>PG: 6. Register providers from DB
        GW->>PG: 7. Wire embedding provider to PGMemoryStore
        GW->>PG: 8. Backfill memory embeddings (background)
    else Standalone mode
        GW->>GW: 4. Create file-based stores
    end

    GW->>GW: 9. Register config-based providers
    GW->>GW: 10. Create tool registry (filesystem, exec, web, memory, browser, TTS, subagent, MCP)
    GW->>GW: 11. Load bootstrap files (DB or filesystem)
    GW->>GW: 12. Create skills loader + register skill_search tool
    GW->>GW: 13. Wire skill embeddings (managed only)

    alt Managed mode
        GW->>GW: 14. Create agents lazily (set ManagedResolver)
        GW->>GW: 15. wireManagedExtras (interceptors, cache subscribers)
        GW->>GW: 16. Wire managed HTTP handlers (agents, skills, traces, MCP)
    else Standalone mode
        GW->>GW: 14. Create agents eagerly from config
    end

    GW->>Engine: 17. Create gateway server (WS + HTTP)
    GW->>Engine: 18. Register RPC methods
    GW->>Engine: 19. Register + start channels (Telegram, Discord, Feishu, Zalo, WhatsApp)
    GW->>Engine: 20. Start cron, heartbeat, scheduler (3 lanes)
    GW->>Engine: 21. Start skills watcher + inbound consumer
    GW->>Engine: 22. Listen on host:port
```

---

## 7. Managed Mode Wiring

The `wireManagedExtras()` function in `cmd/gateway_managed.go` performs 7 steps to wire multi-tenant components:

```mermaid
flowchart TD
    W1["1. ContextFileInterceptor<br/>Routes read_file / write_file to DB"] --> W2
    W2["2. User Seeding Callback<br/>Seeds per-user context files on first chat"] --> W3
    W3["3. Context File Loader<br/>Loads per-user vs agent-level files by agent_type"] --> W4
    W4["4. ManagedResolver<br/>Lazy-creates agent Loops from DB on cache miss"] --> W5
    W5["5. Virtual FS Interceptors<br/>Wire interceptors on read_file + write_file + memory tools"] --> W6
    W6["6. Memory Store Wiring<br/>Wire PGMemoryStore on memory_search + memory_get tools"] --> W7
    W7["7. Cache Invalidation Subscribers<br/>Subscribe to MessageBus events"]
```

### Cache Invalidation Events

| Event | Subscriber | Action |
|-------|-----------|--------|
| `cache:bootstrap` | ContextFileInterceptor | `InvalidateAgent()` or `InvalidateAll()` |
| `cache:agent` | AgentRouter | `InvalidateAgent()` -- forces re-resolve from DB |
| `cache:skills` | SkillStore | `BumpVersion()` |
| `cache:cron` | CronStore | `InvalidateCache()` |
| `cache:custom_tools` | DynamicToolLoader | `ReloadGlobal()` + `AgentRouter.InvalidateAll()` |

---

## 8. Scheduler Lanes

The scheduler uses a lane-based concurrency model. Each lane is a named worker pool with a bounded semaphore. Per-session serialization ensures only one agent run executes at a time per session key.

```mermaid
flowchart TD
    subgraph Main["Lane: main (concurrency 2)"]
        M1[Channel messages]
        M2[WebSocket requests]
    end

    subgraph Sub["Lane: subagent (concurrency 4)"]
        S1[Subagent executions]
    end

    subgraph Cron["Lane: cron (concurrency 1)"]
        C1[Cron job executions]
    end

    Main --> SEM1[Semaphore]
    Sub --> SEM2[Semaphore]
    Cron --> SEM3[Semaphore]

    SEM1 --> Q[Per-Session Queue<br/>1 run at a time per session]
    SEM2 --> Q
    SEM3 --> Q

    Q --> AGENT[Agent Loop]
```

### Queue Modes

| Mode | Behavior |
|------|----------|
| `queue` | FIFO -- new messages wait until the current run completes |
| `followup` | Merges incoming message into the pending queue as a follow-up |
| `interrupt` | Cancels the active run and replaces it with the new message |

Default queue config: capacity 10, drop policy `old` (drops oldest on overflow), debounce 800ms.

---

## 9. Graceful Shutdown

When the process receives SIGINT or SIGTERM:

1. Broadcast `shutdown` event to all connected WebSocket clients.
2. `channelMgr.StopAll()` -- stop all channel adapters.
3. `cronStore.Stop()` -- stop cron scheduler.
4. `heartbeatSvc.Stop()` -- stop heartbeat service.
5. `sandboxMgr.Stop()` + `ReleaseAll()` -- release Docker containers.
6. `cancel()` -- cancel root context, propagating to consumer + scheduler.
7. Deferred cleanup: flush tracing collector, close memory store, close browser manager, stop scheduler lanes.
8. HTTP server shutdown with a **5-second timeout** (`context.WithTimeout`).

---

## 10. Config System

Configuration is loaded from a JSON5 file with environment variable overlay. Secrets are never persisted to the config file.

```mermaid
flowchart TD
    A{Config path?} -->|--config flag| B[CLI flag path]
    A -->|GOCLAW_CONFIG env| C[Env var path]
    A -->|default| D["config.json"]

    B & C & D --> LOAD["config.Load()"]
    LOAD --> S1["1. Set defaults"]
    S1 --> S2["2. Parse JSON5"]
    S2 --> S3["3. Env var overlay<br/>(GOCLAW_*_API_KEY)"]
    S3 --> S4["4. Apply computed defaults<br/>(context pruning, etc.)"]
    S4 --> READY[Config ready]
```

### Key Config Sections

| Section | Purpose |
|---------|---------|
| `gateway` | host, port, token, allowed_origins, rate_limit_rpm, max_message_chars |
| `agents` | defaults (provider, model, context_window) + list (per-agent overrides) |
| `tools` | profile, allow/deny lists, exec_approval, web, browser, mcp_servers, rate_limit_per_hour |
| `channels` | Per-channel: enabled, token, dm_policy, group_policy, allow_from |
| `database` | mode (standalone/managed); postgres_dsn read only from env var |

### Secret Handling

- Secrets exist only in env vars or `.env.local` -- never in `config.json`.
- `GOCLAW_POSTGRES_DSN` is tagged `json:"-"` and cannot be read from the config file.
- `MaskedCopy()` replaces API keys with `"***"` when returning config over WebSocket.
- `StripSecrets()` removes secrets before writing config to disk.
- Config hot-reload via `fsnotify` watcher with 300ms debounce.

---

## 11. File Reference

| File | Purpose |
|------|---------|
| `cmd/root.go` | Cobra CLI entry point, flag parsing |
| `cmd/gateway.go` | Gateway startup orchestrator (`runGateway()`) |
| `cmd/gateway_managed.go` | Managed mode wiring (`wireManagedExtras()`, `wireManagedHTTP()`) |
| `cmd/gateway_providers.go` | Provider registration (config-based + DB-based) |
| `cmd/gateway_methods.go` | RPC method registration |
| `internal/config/config.go` | Config struct definitions |
| `internal/config/config_load.go` | JSON5 loading + env overlay |
| `internal/config/config_channels.go` | Channel config structs |
| `internal/gateway/server.go` | WS + HTTP server, CORS, rate limiter setup |
| `internal/gateway/client.go` | WebSocket client handling, read limit (512KB) |
| `internal/gateway/router.go` | RPC method routing |
| `internal/scheduler/lanes.go` | Lane definitions, semaphore-based concurrency |
| `internal/scheduler/queue.go` | Per-session queue, queue modes, debounce |
| `internal/store/stores.go` | `Stores` container struct (all store interfaces) |
| `internal/store/types.go` | `StoreConfig`, `BaseModel` |

---

## Cross-References

| Document | Content |
|----------|---------|
| [01-agent-loop.md](./01-agent-loop.md) | Agent loop detail, sanitization pipeline, history management |
| [02-providers.md](./02-providers.md) | LLM providers, retry logic, schema cleaning |
| [03-tools-system.md](./03-tools-system.md) | Tool registry, policy engine, interceptors, custom tools, MCP grants |
| [04-gateway-protocol.md](./04-gateway-protocol.md) | WebSocket protocol v3, HTTP API, RBAC, identity propagation |
| [05-channels-messaging.md](./05-channels-messaging.md) | Channel adapters, Telegram formatting, pairing, managed-mode user scoping |
| [06-store-data-model.md](./06-store-data-model.md) | Store interfaces, PostgreSQL schema, session caching, custom tool store |
| [07-bootstrap-skills-memory.md](./07-bootstrap-skills-memory.md) | Bootstrap files, skills system, memory, skills grants |
| [08-scheduling-cron-heartbeat.md](./08-scheduling-cron-heartbeat.md) | Scheduler lanes, cron lifecycle, heartbeat |
| [09-security.md](./09-security.md) | Defense layers, encryption, rate limiting, RBAC, sandbox |
| [10-tracing-observability.md](./10-tracing-observability.md) | Tracing collector, span hierarchy, OTel export, trace API |
