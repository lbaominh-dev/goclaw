<p align="center">
  <img src="_statics/goclaw.png" alt="GoClaw" />
</p>

# GoClaw

[![Go](https://img.shields.io/badge/Go_1.25-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/) [![PostgreSQL](https://img.shields.io/badge/PostgreSQL_15+-316192?style=flat-square&logo=postgresql&logoColor=white)](https://www.postgresql.org/) [![Docker](https://img.shields.io/badge/Docker-2496ED?style=flat-square&logo=docker&logoColor=white)](https://www.docker.com/) [![WebSocket](https://img.shields.io/badge/WebSocket-010101?style=flat-square&logo=socket.io&logoColor=white)](https://developer.mozilla.org/en-US/docs/Web/API/WebSocket) [![OpenTelemetry](https://img.shields.io/badge/OpenTelemetry-000000?style=flat-square&logo=opentelemetry&logoColor=white)](https://opentelemetry.io/) [![Anthropic](https://img.shields.io/badge/Anthropic-191919?style=flat-square&logo=anthropic&logoColor=white)](https://www.anthropic.com/) [![OpenAI](https://img.shields.io/badge/OpenAI_Compatible-412991?style=flat-square&logo=openai&logoColor=white)](https://openai.com/) [![License: MIT](https://img.shields.io/badge/License-MIT-yellow?style=flat-square)](LICENSE)

> Multi-agent AI gateway with WebSocket RPC, HTTP API, and messaging channel integrations.
> A Go port of [OpenClaw](https://github.com/openclaw/openclaw) with enhanced security, multi-tenant PostgreSQL, and production-grade observability.

## Why GoClaw?

GoClaw is OpenClaw, reimagined in Go. It preserves the powerful gateway architecture while delivering a single compiled binary with no Node.js runtime, defense-in-depth security, and PostgreSQL-native multi-tenancy with per-user workspaces.

## Key Improvements over OpenClaw

### Security Hardening

- **Rate limiting** — Token bucket per user/IP via `golang.org/x/time/rate`, configurable RPM
- **Prompt injection detection** — 6-pattern regex scanner (instruction override, role injection, system tags, etc.)
- **Credential scrubbing** — Auto-redact API keys, tokens, and passwords from tool outputs
- **Shell deny patterns** — Blocks `curl|sh`, reverse shells, `eval $()`, `base64|sh`
- **SSRF protection** — DNS pinning, blocked private IPs, blocked hosts
- **AES-256-GCM** — Encrypted API keys in database (managed mode)

### Skill System

- **BM25 full-text search** indexing for fast skill discovery
- **Embedding-based search** — Hybrid BM25 + vector search via pgvector (managed mode)
- **ZIP upload** with SKILL.md frontmatter validation and version tracking
- **Fine-grained grants** — Agent-level and user-level access control

### Parallel Execution

- **Lane-based scheduler** — Main / subagent / delegate / cron lane isolation (env-configurable concurrency)
- **Concurrent agent runs** — Multiple users in a group chat get parallel responses (configurable `maxConcurrent`, default 3 for groups). Adaptive throttle reduces concurrency when session history nears summarization threshold
- **Session history isolation** — Concurrent runs buffer messages locally and flush atomically on completion, preventing cross-run context pollution
- **Concurrent subagents** — Depth limits, count limits, model override
- **Batched announce queue** — Debounced result delivery

### Multi-User Managed Mode (PostgreSQL)

Standalone mode shares everything across users (same as OpenClaw). Managed mode adds full per-user isolation:

- **Per-user context files** — 7 files for open agents (AGENTS, SOUL, TOOLS, IDENTITY, USER, HEARTBEAT, BOOTSTRAP)
- **Agent types** — `open` (per-user workspace) vs `predefined` (shared context + USER.md)
- **User tracking** — first_seen_at, last_seen_at, workspace per user-agent pair
- **Per-user overrides** — Provider and model overrides per user

### Single Binary Distribution

- No Node.js runtime required
- `CGO_ENABLED=0` static binary (~25 MB base, ~36 MB with OTel)
- Alpine Docker image (~50 MB)

### Production Observability

- **OpenTelemetry OTLP** export (gRPC/HTTP), opt-in via build tag
- **Verbose tracing** — Full LLM input logging in trace spans (`GOCLAW_TRACE_VERBOSE=1`)
- **Compile-out OTel** — Remove import to save ~11 MB binary size

### Claw Ecosystem

**Resource Footprint:**

|                 | OpenClaw        | ZeroClaw | PicoClaw | **GoClaw**                              |
| --------------- | --------------- | -------- | -------- | --------------------------------------- |
| Language        | TypeScript      | Rust     | Go       | **Go**                                  |
| Binary size     | 28 MB + Node.js | 3.4 MB   | ~8 MB    | **~25 MB** (base) / **~36 MB** (+ OTel) |
| Docker image    | —               | —        | —        | **~50 MB** (Alpine)                     |
| RAM (idle)      | > 1 GB          | < 5 MB   | < 10 MB  | **~35 MB**                              |
| Startup         | > 5 s           | < 10 ms  | < 1 s    | **< 1 s**                               |
| Target hardware | $599+ Mac Mini  | $10 edge | $10 edge | **$5 VPS+**                             |

**Feature Matrix:**

| Feature                    | OpenClaw                             | ZeroClaw                                     | PicoClaw                              | **GoClaw**                     |
| -------------------------- | ------------------------------------ | -------------------------------------------- | ------------------------------------- | ------------------------------ |
| Multi-tenant (PostgreSQL)  | —                                    | —                                            | —                                     | ✅                             |
| Custom tools (runtime API) | Config-based only                    | —                                            | —                                     | ✅                             |
| MCP integration            | — (uses ACP)                         | —                                            | —                                     | ✅ (stdio/SSE/streamable-http) |
| Security hardening         | ✅ (SSRF, path traversal, injection) | ✅ (sandbox, rate limit, injection, pairing) | Basic (workspace restrict, exec deny) | ✅ 5-layer defense             |
| OTel observability         | ✅ (opt-in extension)                | ✅ (Prometheus + OTLP)                       | —                                     | ✅ OTLP (opt-in build tag)     |
| Skill system               | ✅ Embeddings/semantic               | ✅ SKILL.md + TOML                           | ✅ Basic                              | ✅ BM25 + pgvector hybrid      |
| Lane-based scheduler       | ✅                                   | Bounded concurrency                          | —                                     | ✅ (main/subagent/cron + concurrent group runs) |
| Messaging channels         | 37+                                  | 15+                                          | 10+                                   | 5+                             |
| Companion apps             | macOS, iOS, Android                  | Python SDK                                   | —                                     | Web dashboard                  |
| Live Canvas / Voice        | ✅ (A2UI + TTS/STT)                  | —                                            | Voice transcription                   | TTS (4 providers)              |
| LLM providers              | 10+                                  | 8 native + 29 compat                         | 13+                                   | **11+**                        |
| Per-user workspaces        | ✅ (file-based)                      | —                                            | —                                     | ✅ (managed mode only)         |
| Encrypted secrets          | — (env vars only)                    | ✅ ChaCha20-Poly1305                         | — (plaintext JSON)                    | ✅ AES-256-GCM in DB           |

> **GoClaw unique strengths:** Only project with multi-tenant PostgreSQL, runtime custom tools via API, and MCP protocol support. Only DB-backed secret encryption (vs ZeroClaw's file-based).

## Features

- **Multi-provider LLM support** — OpenRouter, Anthropic, OpenAI, Groq, DeepSeek, Gemini, Mistral, xAI, MiniMax, Cohere, Perplexity, and any OpenAI-compatible endpoint
- **Agent loop** — Think-act-observe cycle with tool use, session history, and auto-summarization
- **Subagents** — Spawn child agents with different models for parallel task execution
- **Agent delegation** — Delegate tasks between named agents with permission-controlled links, sync/async modes, concurrency limits, per-user restrictions, and hybrid FTS + semantic agent discovery (managed mode)
- **Messaging channels** — Telegram, Discord, Zalo, Feishu/Lark, WhatsApp. `/stop` cancels the current task, `/stopall` cancels all running tasks in a chat
- **Memory system** — Long-term memory with SQLite FTS5 + vector embeddings (standalone) or pgvector hybrid search (managed)
- **Skills** — SKILL.md-based knowledge base with BM25 search + embedding hybrid search (managed mode)
- **Custom tools** — Define shell-based tools at runtime via HTTP API with JSON Schema parameters, auto shell-escaping, and encrypted environment variables (managed mode)
- **MCP integration** — Connect external MCP servers via stdio, SSE, or streamable-http transports with per-agent/per-user grants and tool name prefixing
- **Cron scheduling** — `at`, `every`, and cron expression syntax for scheduled agent tasks
- **Browser automation** — Headless Chrome via Rod for web interaction
- **Text-to-Speech** — OpenAI, ElevenLabs, Edge, MiniMax providers
- **Docker sandbox** — Isolated code execution in containers
- **Tracing** — LLM call tracing with optional OpenTelemetry OTLP export
- **Security hardening** — Rate limiting, input guard, CORS, shell deny patterns, SSRF protection, path traversal prevention
- **Browser pairing** — Token-free browser authentication with admin-approved pairing codes
- **Tailscale integration** — Optional secure remote access via Tailscale VPN mesh (build-tag gated, no binary bloat in default build)
- **Web dashboard** — React admin UI for agents, traces, and skills

## Quick Start

### From Source

```bash
# Build
go build -o goclaw .

# Interactive setup wizard
./goclaw onboard

# Start the gateway
source .env.local && ./goclaw
```

### With Docker

```bash
# Create .env with your API key
echo "GOCLAW_OPENROUTER_API_KEY=sk-or-your-key" > .env

# Standalone mode (file-based storage)
docker compose -f docker-compose.yml -f docker-compose.standalone.yml up

# Managed mode (PostgreSQL)
docker compose -f docker-compose.yml -f docker-compose.managed.yml up

# Managed mode + OpenTelemetry tracing
docker compose -f docker-compose.yml -f docker-compose.managed.yml -f docker-compose.otel.yml up

# Managed mode + Tailscale (secure remote access)
docker compose -f docker-compose.yml -f docker-compose.managed.yml -f docker-compose.tailscale.yml up
```

## Installation

### Prerequisites

- Go 1.25+
- PostgreSQL 15+ with pgvector (managed mode only)
- Docker (optional, for sandbox and containerized deployment)

### Build

```bash
# Production build (~25MB binary, static, stripped symbols)
CGO_ENABLED=0 go build -ldflags="-s -w" -o goclaw .

# With OpenTelemetry support (~36MB binary)
CGO_ENABLED=0 go build -ldflags="-s -w" -tags otel -o goclaw .

# With Tailscale support (~54MB binary)
CGO_ENABLED=0 go build -ldflags="-s -w" -tags tsnet -o goclaw .

# With both OTel + Tailscale
CGO_ENABLED=0 go build -ldflags="-s -w" -tags "otel,tsnet" -o goclaw .
```

**Binary size comparison across the Claw ecosystem:**

| Build                    | Binary Size | Docker Image | Notes                                     |
| ------------------------ | ----------- | ------------ | ----------------------------------------- |
| **GoClaw** (base)        | ~25 MB      | ~50 MB       | `CGO_ENABLED=0 go build -ldflags="-s -w"` |
| **GoClaw** (+ OTel)      | ~36 MB      | ~60 MB       | Add `-tags otel` for OTLP export          |
| **GoClaw** (+ Tailscale) | ~54 MB      | ~75 MB       | Add `-tags tsnet` for Tailscale listener  |
| **GoClaw** (+ both)      | ~65 MB      | ~85 MB       | `-tags "otel,tsnet"`                      |
| PicoClaw                 | ~8 MB       | —            | Single Go binary                          |
| ZeroClaw                 | 3.4 MB      | —            | Minimal Rust binary                       |
| OpenClaw                 | 28 MB       | —            | + ~390 MB Node.js runtime required        |

> Optional features are gated behind build tags to avoid binary bloat. OTel adds ~11 MB (gRPC + protobuf). Tailscale adds ~20 MB (tsnet + WireGuard). The base build includes in-app tracing backed by PostgreSQL and localhost-only access.

### Docker Build

```bash
# Standard image (~50MB Alpine)
docker build -t goclaw .

# With OpenTelemetry (~60MB)
docker build --build-arg ENABLE_OTEL=true -t goclaw:otel .

# With Tailscale (~75MB)
docker build --build-arg ENABLE_TSNET=true -t goclaw:tsnet .

# With both OTel + Tailscale (~85MB)
docker build --build-arg ENABLE_OTEL=true --build-arg ENABLE_TSNET=true -t goclaw:full .
```

## Configuration

### Setup Wizard

```bash
./goclaw onboard
```

The wizard configures: provider, model, gateway port, channels, memory, browser, TTS, tracing, and database mode. It generates `config.json` (no secrets) and `.env.local` (secrets only).

### Auto-Onboard (Docker / CI)

When `GOCLAW_*_API_KEY` environment variables are set, the gateway automatically configures itself without interactive prompts. In managed mode, it retries Postgres connection (up to 5 attempts), runs migrations, and seeds default data.

### Environment Variables

**Provider API Keys** (set at least one):

| Variable                    | Provider                 |
| --------------------------- | ------------------------ |
| `GOCLAW_OPENROUTER_API_KEY` | OpenRouter (recommended) |
| `GOCLAW_ANTHROPIC_API_KEY`  | Anthropic Claude         |
| `GOCLAW_OPENAI_API_KEY`     | OpenAI                   |
| `GOCLAW_GROQ_API_KEY`       | Groq                     |
| `GOCLAW_DEEPSEEK_API_KEY`   | DeepSeek                 |
| `GOCLAW_GEMINI_API_KEY`     | Google Gemini            |
| `GOCLAW_MISTRAL_API_KEY`    | Mistral AI               |
| `GOCLAW_XAI_API_KEY`        | xAI Grok                 |
| `GOCLAW_MINIMAX_API_KEY`    | MiniMax                  |
| `GOCLAW_COHERE_API_KEY`     | Cohere                   |
| `GOCLAW_PERPLEXITY_API_KEY` | Perplexity               |

**Gateway & Application:**

| Variable                  | Description                      | Default                      |
| ------------------------- | -------------------------------- | ---------------------------- |
| `GOCLAW_CONFIG`           | Config file path                 | `config.json`                |
| `GOCLAW_GATEWAY_TOKEN`    | API authentication token         | (generated)                  |
| `GOCLAW_HOST`             | Server bind address              | `0.0.0.0`                    |
| `GOCLAW_PORT`             | Server port                      | `18790`                      |
| `GOCLAW_PROVIDER`         | Default LLM provider             | `anthropic`                  |
| `GOCLAW_MODEL`            | Default model                    | `claude-sonnet-4-5-20250929` |
| `GOCLAW_WORKSPACE`        | Agent workspace directory        | `~/.goclaw/workspace`        |
| `GOCLAW_DATA_DIR`         | Data storage directory           | `~/.goclaw/data`             |
| `GOCLAW_SESSIONS_STORAGE` | Sessions storage path            | `~/.goclaw/sessions`         |
| `GOCLAW_SKILLS_DIR`       | Skills directory                 | `~/.goclaw/skills`           |
| `GOCLAW_OWNER_IDS`        | Owner user IDs (comma-separated) |                              |

**Database (Managed Mode):**

| Variable                | Description                            |
| ----------------------- | -------------------------------------- |
| `GOCLAW_MODE`           | `standalone` or `managed`              |
| `GOCLAW_POSTGRES_DSN`   | PostgreSQL connection string           |
| `GOCLAW_ENCRYPTION_KEY` | AES-256-GCM key for API key encryption |
| `GOCLAW_MIGRATIONS_DIR` | Path to migration files                |

**Messaging Channels:**

| Variable                           | Description                   |
| ---------------------------------- | ----------------------------- |
| `GOCLAW_TELEGRAM_TOKEN`            | Telegram bot token            |
| `GOCLAW_ZALO_TOKEN`                | Zalo access token             |
| `GOCLAW_FEISHU_APP_ID`             | Feishu/Lark app ID            |
| `GOCLAW_FEISHU_APP_SECRET`         | Feishu/Lark app secret        |
| `GOCLAW_FEISHU_ENCRYPT_KEY`        | Feishu message encryption key |
| `GOCLAW_FEISHU_VERIFICATION_TOKEN` | Feishu verification token     |

**Scheduler Lanes:**

| Variable               | Description                  | Default |
| ---------------------- | ---------------------------- | ------- |
| `GOCLAW_LANE_MAIN`     | Main lane concurrency        | `30`    |
| `GOCLAW_LANE_SUBAGENT` | Subagent lane concurrency    | `50`    |
| `GOCLAW_LANE_DELEGATE` | Delegation lane concurrency  | `100`   |
| `GOCLAW_LANE_CRON`     | Cron lane concurrency        | `30`    |

**Tailscale (requires build tag `tsnet`):**

| Variable                | Description                                   | Default    |
| ----------------------- | --------------------------------------------- | ---------- |
| `GOCLAW_TSNET_HOSTNAME` | Tailscale device name (e.g. `goclaw-gateway`) | (disabled) |
| `GOCLAW_TSNET_AUTH_KEY` | Tailscale auth key                            |            |
| `GOCLAW_TSNET_DIR`      | Persistent state directory                    | OS default |

**Telemetry (requires build tag `otel`):**

| Variable                        | Description                 | Default          |
| ------------------------------- | --------------------------- | ---------------- |
| `GOCLAW_TELEMETRY_ENABLED`      | Enable OTel export          | `false`          |
| `GOCLAW_TELEMETRY_ENDPOINT`     | OTLP endpoint               |                  |
| `GOCLAW_TELEMETRY_PROTOCOL`     | `grpc` or `http`            | `grpc`           |
| `GOCLAW_TELEMETRY_INSECURE`     | Skip TLS verification       | `false`          |
| `GOCLAW_TELEMETRY_SERVICE_NAME` | Service name in traces      | `goclaw-gateway` |
| `GOCLAW_TRACE_VERBOSE`          | Log full LLM input in spans | `0`              |

**TTS (Text-to-Speech):**

| Variable                        | Description         |
| ------------------------------- | ------------------- |
| `GOCLAW_TTS_OPENAI_API_KEY`     | OpenAI TTS API key  |
| `GOCLAW_TTS_ELEVENLABS_API_KEY` | ElevenLabs API key  |
| `GOCLAW_TTS_MINIMAX_API_KEY`    | MiniMax TTS API key |
| `GOCLAW_TTS_MINIMAX_GROUP_ID`   | MiniMax group ID    |

## Deployment Modes

### Standalone (Default)

File-based storage, no external database required. Behaves like the original OpenClaw — all users share the same workspace, sessions, and context files. There is no per-user isolation; every user sees and modifies the same agent state. Best suited for single-user or trusted-team setups.

```
config.json          -> Non-secret settings
.env.local           -> Secrets (API keys, tokens)
~/.goclaw/
  |-- workspace/     -> Shared agent workspace (SOUL.md, AGENTS.md, etc.)
  |-- data/          -> Cron jobs, pairing data
  |-- sessions/      -> Chat session history (shared across users)
  +-- skills/        -> User-managed skills
```

### Managed (PostgreSQL)

All data in PostgreSQL with pgvector support. Designed for multi-user and multi-tenant deployments with **per-user isolation** — each user gets their own context files, session history, and workspace. This is the key difference from standalone mode (and OpenClaw).

```bash
# Set up database
export GOCLAW_MODE=managed
export GOCLAW_POSTGRES_DSN="postgres://user:pass@localhost:5432/goclaw?sslmode=disable"
export GOCLAW_ENCRYPTION_KEY=$(openssl rand -hex 32)

# Run migrations
./goclaw migrate up

# Start gateway
./goclaw
```

**Managed mode features:**

- Agent definitions stored in `agents` table
- Per-user context files (`user_context_files` table)
- Agent types: `open` (per-user workspace) vs `predefined` (shared context)
- API key encryption (AES-256-GCM)
- LLM call tracing with spans
- MCP server integration with per-agent and per-user access grants
- Embedding-based skill search (hybrid BM25 + pgvector)
- HTTP API for agents, skills, traces, and MCP servers

## CLI Commands

```
goclaw                    Start gateway (default command)
goclaw onboard            Interactive setup wizard
goclaw version            Print version and protocol info
goclaw doctor             System health check

goclaw agent list         List configured agents
goclaw agent chat         Chat with an agent
goclaw agent add          Add a new agent
goclaw agent delete       Delete an agent

goclaw migrate up         Apply all pending migrations
goclaw migrate down       Roll back migrations
goclaw migrate version    Show current migration version
goclaw migrate force N    Force set migration version
goclaw migrate goto N     Migrate to specific version
goclaw migrate drop       Drop all tables (dangerous)

goclaw config show        Show current configuration
goclaw config path        Show config file path
goclaw config validate    Validate configuration

goclaw sessions list      List active sessions
goclaw sessions delete    Delete a session
goclaw sessions reset     Reset session history

goclaw cron list          List scheduled jobs
goclaw cron delete        Delete a job
goclaw cron toggle        Enable/disable a job

goclaw skills list        List available skills
goclaw skills show        Show skill details

goclaw models             List AI models and providers
goclaw channels           List messaging channels

goclaw pairing approve    Approve a pairing code
goclaw pairing list       List paired devices
goclaw pairing revoke     Revoke a pairing
```

**Flags:**

```
--config, -c    Path to config file (default: config.json)
--verbose, -v   Enable debug logging
```

## API

See [API Reference](api-reference.md) for HTTP endpoints, Custom Tools, and MCP Integration.

See [WebSocket Protocol](websocket-protocol.md) for the real-time RPC protocol (v3).

## Docker Compose

Six composable files for different deployment scenarios:

| File                             | Purpose                                            |
| -------------------------------- | -------------------------------------------------- |
| `docker-compose.yml`             | Base service definition                            |
| `docker-compose.standalone.yml`  | File-based storage with persistent volumes         |
| `docker-compose.managed.yml`     | PostgreSQL (pgvector/pgvector:pg18) + managed mode |
| `docker-compose.selfservice.yml` | Web dashboard UI (nginx + React SPA)               |
| `docker-compose.otel.yml`        | OpenTelemetry + Jaeger tracing                     |
| `docker-compose.tailscale.yml`   | Tailscale VPN mesh listener                        |

### Examples

```bash
# Standalone
docker compose -f docker-compose.yml -f docker-compose.standalone.yml up -d

# Managed (PostgreSQL)
docker compose -f docker-compose.yml -f docker-compose.managed.yml up -d

# Managed + Web Dashboard (http://localhost:3000)
docker compose -f docker-compose.yml \
  -f docker-compose.managed.yml \
  -f docker-compose.selfservice.yml up -d

# Managed + Web Dashboard + OpenTelemetry (Jaeger UI at http://localhost:16686)
docker compose -f docker-compose.yml \
  -f docker-compose.managed.yml \
  -f docker-compose.selfservice.yml \
  -f docker-compose.otel.yml up -d

# Managed + Tailscale (secure remote access via VPN mesh)
docker compose -f docker-compose.yml \
  -f docker-compose.managed.yml \
  -f docker-compose.tailscale.yml up -d

# Check health
curl http://localhost:18790/health
```

### Environment File (.env)

Create a `.env` file in the project root for Docker Compose:

```bash
# Required: at least one provider API key
GOCLAW_OPENROUTER_API_KEY=sk-or-your-key

# Optional: gateway token (auto-generated if not set)
GOCLAW_GATEWAY_TOKEN=your-token

# Optional: Postgres credentials (managed mode)
POSTGRES_USER=goclaw
POSTGRES_PASSWORD=your-secure-password
POSTGRES_DB=goclaw
```

## Architecture

```
+-----------------------------------------------------------+
|                     Gateway Server                         |
|  +----------+  +----------+  +------------------------+   |
|  | WebSocket|  |  HTTP API |  |    Channel Manager     |   |
|  |  /ws     |  | /v1/chat  |  | Telegram|Discord|Feishu|   |
|  +----+-----+  +----+-----+  +--------+---------------+   |
|       |              |                 |                    |
|       +--------------+-----------------+                    |
|                      v                                      |
|              +---------------+                              |
|              |  Message Bus  |                              |
|              +-------+-------+                              |
|                      v                                      |
|              +---------------+                              |
|              |   Scheduler   |  (lane-based concurrency)    |
|              +-------+-------+                              |
|                      v                                      |
|  +-----------------------------------------------------+   |
|  |                 Agent Router                         |   |
|  |  +---------+  +---------+  +-----------------+      |   |
|  |  | default |  | agent-2 |  |  subagent (lazy) |     |   |
|  |  +----+----+  +----+----+  +--------+--------+     |   |
|  |       +-------------+---------------+               |   |
|  |                     v                               |   |
|  |           +------------------+                      |   |
|  |           |    Agent Loop    |                      |   |
|  |           | think->act->observe|                    |   |
|  |           +--------+---------+                      |   |
|  +--------------------+-------------------------------+   |
|                       v                                     |
|  +-----------------------------------------------------+  |
|  |              Tool Registry                           |  |
|  | read_file|write_file|exec|web_search|web_fetch|...   |  |
|  | memory|skill_search|tts|spawn|browser|delegate|...   |  |
|  | custom tools (runtime) | MCP bridge (stdio/SSE/HTTP) |  |
|  +-----------------------------------------------------+  |
|                       v                                     |
|  +-----------------------------------------------------+  |
|  |            LLM Provider Registry                     |  |
|  | Anthropic (native HTTP+SSE) | OpenAI-compat (HTTP/SSE) |  |
|  +-----------------------------------------------------+  |
|                       v                                     |
|  +-----------------------------------------------------+  |
|  |                 Store Layer                          |  |
|  |   Standalone: file-based  |  Managed: PostgreSQL     |  |
|  +-----------------------------------------------------+  |
+-----------------------------------------------------------+
```

## Built-in Tools

| Tool               | Group      | Description                                                  |
| ------------------ | ---------- | ------------------------------------------------------------ |
| `read_file`        | fs         | Read file contents (with virtual FS routing in managed mode) |
| `write_file`       | fs         | Write/create files                                           |
| `edit_file`        | fs         | Apply targeted edits to existing files                       |
| `list_files`       | fs         | List directory contents                                      |
| `search`           | fs         | Search file contents by pattern                              |
| `glob`             | fs         | Find files by glob pattern                                   |
| `exec`             | runtime    | Execute shell commands (with approval workflow)              |
| `process`          | runtime    | Manage running processes                                     |
| `web_search`       | web        | Search the web (Brave, DuckDuckGo)                           |
| `web_fetch`        | web        | Fetch and parse web content                                  |
| `memory_search`    | memory     | Search long-term memory (FTS + vector)                       |
| `memory_get`       | memory     | Retrieve memory entries                                      |
| `skill_search`     | —          | Search skills (BM25 + embedding hybrid in managed mode)      |
| `image`            | —          | Image generation/manipulation                                |
| `message`          | messaging  | Send messages to channels                                    |
| `tts`              | —          | Text-to-Speech synthesis                                     |
| `spawn`            | —          | Spawn a subagent                                             |
| `subagents`        | sessions   | Control running subagents                                    |
| `delegate`         | —          | Delegate tasks to other agents (sync/async, cancel, list)    |
| `delegate_search`  | —          | Search delegation targets (hybrid FTS + semantic)            |
| `sessions_list`    | sessions   | List active sessions                                         |
| `sessions_history` | sessions   | View session history                                         |
| `sessions_send`    | sessions   | Send message to a session                                    |
| `sessions_spawn`   | sessions   | Spawn a new session                                          |
| `session_status`   | sessions   | Check session status                                         |
| `cron`             | automation | Schedule and manage cron jobs                                |
| `gateway`          | automation | Gateway administration                                       |
| `browser`          | ui         | Browser automation (navigate, click, type, screenshot)       |
| `canvas`           | ui         | Visual canvas for diagrams                                   |

## Agent Delegation (Managed Mode)

Agent delegation enables named agents to delegate tasks to other agents — each running with its own identity, tools, LLM provider, and context files. Unlike subagents (anonymous clones of the parent), delegation targets are fully independent agents with their own expertise.

### How Agents Collaborate

```mermaid
flowchart TD
    USER((👤 User)) -->|"Research competitor pricing"| SUPPORT

    subgraph TEAM["🤖 Agent Team"]
        SUPPORT["💬 Support Bot\n(Claude Haiku)"]
        RESEARCH["🔍 Research Bot\n(GPT-4)"]
        WRITER["✍️ Content Writer\n(Claude Sonnet)"]
        BILLING["💳 Billing Bot\n(Gemini)"]
    end

    SUPPORT -->|"🔄 sync: wait for answer"| RESEARCH
    RESEARCH -->|"📋 result"| SUPPORT
    SUPPORT -->|"⚡ async: don't wait"| WRITER
    WRITER -.->|"📣 announce when done"| SUPPORT
    SUPPORT -.-x|"🚫 no link"| BILLING

    SUPPORT -->|"✅ final answer"| USER

    style USER fill:#e1f5fe
    style SUPPORT fill:#fff3e0
    style RESEARCH fill:#e8f5e9
    style WRITER fill:#f3e5f5
    style BILLING fill:#ffebee
```

| Mode | How it works | Best for |
|------|-------------|----------|
| **Sync** | Agent A asks Agent B and **waits** for the answer | Quick lookups, fact checks |
| **Async** | Agent A asks Agent B and **moves on**. B announces the result later | Long tasks, reports, deep analysis |

### Architecture

```mermaid
flowchart TD
    U[User] -->|chat| A[Agent A<br/>Customer Support<br/>Claude Haiku]

    subgraph Delegation["Inter-Agent Delegation"]
        A -->|"delegate(sync)"| B[Agent B<br/>Research Bot<br/>GPT-4]
        A -->|"delegate(async)"| C[Agent C<br/>Content Writer<br/>Claude Sonnet]
        B -->|result| A
        C -->|announce via Message Bus| A
    end

    subgraph Permission["Permission Layer"]
        LINKS[(agent_links<br/>direction · max_concurrent<br/>settings JSONB)]
        LINKS -->|outbound| B
        LINKS -->|bidirectional| C
    end

    subgraph Search["Agent Discovery"]
        FTS[FTS<br/>tsvector on agents]
        VEC[Semantic<br/>pgvector embeddings]
        FTS & VEC -->|hybrid 0.3 + 0.7| RESULTS[delegate_search tool]
    end

    A -.->|"≤15 targets"| AGENTSMD["AGENTS.md<br/>(auto-injected)"]
    A -.->|">15 targets"| RESULTS
```

### Permission Links

Agents communicate through explicit **agent links** — directed edges with access control:

```bash
# Create a one-way link: support-bot can delegate TO research-bot
agents.links.create {
  "sourceAgent": "support-bot",
  "targetAgent": "research-bot",
  "direction": "outbound",
  "description": "Allow support to request research",
  "maxConcurrent": 3
}

# Bidirectional: both agents can delegate to each other
agents.links.create {
  "sourceAgent": "support-bot",
  "targetAgent": "content-writer",
  "direction": "bidirectional"
}
```

| Direction | Meaning |
|-----------|---------|
| `outbound` | Source can delegate TO target |
| `inbound` | Target can delegate TO source |
| `bidirectional` | Both agents can delegate to each other |

### Delegation Modes

**Sync** (default) — Agent A blocks while B processes:

```
delegate(agent="research-bot", task="Find competitor pricing for X")
→ research-bot runs full agent loop (own tools, identity, provider)
→ result returned inline to agent A
```

**Async** — Agent A continues immediately, result announced later:

```
delegate(agent="research-bot", task="Deep market analysis", mode="async")
→ returns immediately with delegation ID
→ research-bot runs in background via "delegate" scheduler lane
→ result announced back through message bus → A reformulates for user
```

**Cancel & List:**

```
delegate(action="cancel", delegation_id="abc123")
delegate(action="list")
```

### Concurrency Control

Two layers prevent any agent from being overwhelmed:

| Layer | Config | Example |
|-------|--------|---------|
| **Per-link** | `agent_links.max_concurrent` | support → research: max 3 |
| **Per-agent** | `agents.other_config.max_delegation_load` | research-bot: max 5 total |

When limits are hit, the delegate tool returns a descriptive error so the LLM can adapt (retry later, use a different agent, or handle the task itself).

### Per-User Restrictions

The `settings` JSONB on agent links supports per-user access control:

```json
{
  "user_deny": ["free-tier-user"],
  "user_allow": ["premium-user-1", "premium-user-2"]
}
```

Empty settings = all users allowed (default). Deny list is checked first, then allow list.

### Agent Discovery

Each agent has a `frontmatter` field — a short expertise summary used for discovery:

- **≤15 delegation targets**: Auto-generated `AGENTS.md` is injected into the agent's context, listing all available targets with usage instructions
- **>15 targets**: Agent is instructed to use the `delegate_search` tool for hybrid FTS + semantic search (BM25 weight 0.3 + pgvector cosine weight 0.7)

```
delegate_search(query="billing payment refund")
→ returns ranked list of matching agents with frontmatter
→ agent picks the best match and delegates
```

### Key Differences from Subagents

| Aspect | Subagents | Agent Delegation |
|--------|-----------|-----------------|
| Target | Anonymous clone of parent | Named agent with own identity |
| Provider/Model | Inherited from parent | Target's own configuration |
| Tools | Parent's tools minus deny list | Target's own tool registry + policy |
| Context files | Simplified system prompt | Target's own SOUL.md, IDENTITY.md, etc. |
| Session | Shared with parent | Isolated (fresh per delegation) |
| Permission | Depth-based limits only | Explicit `agent_links` with direction |
| User control | None | Per-user deny/allow via settings JSONB |
| Concurrency | Global + per-parent limits | Per-link + per-target-agent limits |

## Browser Pairing

Browser clients can authenticate without pre-shared tokens using a pairing code flow:

1. User opens the web dashboard and enters their User ID
2. Clicks "Request Access (Pairing)" — gateway generates an 8-character code
3. Code is displayed in the browser UI
4. An admin approves the code via CLI (`goclaw pairing approve XXXX`) or the web UI
5. Browser automatically detects approval and gains operator-level access
6. On subsequent visits, the browser reconnects automatically using the stored pairing (no re-approval needed)

**Revoking access:**

```bash
# List paired devices
goclaw pairing list

# Revoke a specific pairing
goclaw pairing revoke <sender_id>
```

After revocation, the browser falls back to the pairing flow on next visit.

## Tailscale (Remote Access)

GoClaw supports an optional [Tailscale](https://tailscale.com) listener for secure remote access via VPN mesh. The Tailscale listener runs alongside the main gateway, serving the same routes on both listeners.

**Build-tag gated:** The `tsnet` dependency (~20 MB) is only compiled when building with `-tags tsnet`. The default binary is unaffected.

```bash
# Build with Tailscale support
go build -tags tsnet -o goclaw .

# Configure via environment variables
export GOCLAW_TSNET_HOSTNAME=goclaw-gateway
export GOCLAW_TSNET_AUTH_KEY=tskey-auth-xxxxx

# Start — both localhost:18790 and Tailscale listener are active
./goclaw
```

When Tailscale is enabled and the gateway is still bound to `0.0.0.0`, a log suggestion recommends switching to `127.0.0.1` for localhost-only + Tailscale access:

```
GOCLAW_HOST=127.0.0.1 ./goclaw
```

This keeps the gateway inaccessible from the LAN while remaining reachable via Tailscale from any device on your tailnet.

**Docker:**

```bash
docker compose -f docker-compose.yml \
  -f docker-compose.managed.yml \
  -f docker-compose.tailscale.yml up -d
```

Requires `GOCLAW_TSNET_AUTH_KEY` in your `.env` file. Tailscale state is persisted in a `tsnet-state` Docker volume.

## Security

- **Transport**: WebSocket CORS validation, 512KB message limit, 1MB HTTP body limit, timing-safe token auth
- **Rate limiting**: Token bucket per user/IP, configurable RPM
- **Prompt injection**: Input guard with 6 pattern detection (detection-only, never blocks)
- **Shell security**: Deny patterns for `curl|sh`, `wget|sh`, reverse shells, `eval`, `base64|sh`
- **Network**: SSRF protection with blocked hosts + private IP + DNS pinning
- **File system**: Path traversal prevention, workspace restriction
- **Encryption**: AES-256-GCM for API keys in database (managed mode)
- **Browser pairing**: Token-free browser auth with admin approval (pairing codes, auto-reconnect)
- **Tailscale**: Optional VPN mesh listener for secure remote access (build-tag gated)

## Testing

```bash
# Unit tests
go test ./...

# Integration tests (requires running gateway)
go test -v -run 'TestHealthHTTP|TestConnectHandshake' ./tests/integration/

# Full integration (requires API key)
GOCLAW_OPENROUTER_API_KEY=sk-or-xxx go test -v ./tests/integration/ -timeout 120s
```

## Project Status

### Implemented & Tested in Production

- **Agent management & configuration** — Create, update, delete agents via API and web dashboard. Agent types (`open` / `predefined`), agent routing, and lazy resolution all tested.
- **Telegram channel** — Full integration tested: message handling, streaming responses, rich formatting (HTML, tables, code blocks), reactions, media, chunked long messages.
- **Seed data & bootstrapping** — Auto-onboard, DB seeding, migration pipeline tested end-to-end in managed mode.
- **User-scope & content files** — Per-user context files (`user_context_files`), agent-level context files (`agent_context_files`), virtual FS interceptors, per-user seeding (`SeedUserFiles`), and user-agent profile tracking all implemented and tested.
- **Core built-in tools** — File system tools (`read_file`, `write_file`, `edit_file`, `list_files`, `search`, `glob`), shell execution (`exec`), web tools (`web_search`, `web_fetch`), and session management tools tested in real agent loops.
- **Memory system** — Long-term memory with search (FTS5 in standalone, pgvector hybrid in managed mode) implemented and tested with real conversations.
- **Agent loop** — Think-act-observe cycle, tool use, session history, auto-summarization, and subagent spawning tested in production.
- **WebSocket RPC protocol (v3)** — Connect handshake, chat streaming, event push all tested with web dashboard and integration tests.
- **Store layer (PostgreSQL)** — All PG stores (sessions, agents, providers, skills, cron, pairing, tracing, memory) implemented and running in managed mode.
- **Browser automation** — Rod/CDP integration for headless Chrome, tested in production agent workflows.
- **Lane-based scheduler** — Main/subagent/delegate/cron lane isolation with concurrent execution tested. Group chats support up to 3 concurrent agent runs per session with adaptive throttle and deferred session writes for history isolation.
- **Security hardening** — Rate limiting, prompt injection detection, CORS, shell deny patterns, SSRF protection, credential scrubbing all implemented and verified.
- **Web dashboard (core)** — Channel management, agent management, pairing approval, traces & spans viewer all implemented and working well.

### Implemented but Not Fully Tested

- **Agent delegation** — Inter-agent task delegation with permission links, sync/async modes, per-user restrictions, concurrency limits, and hybrid agent search. Core implementation complete, needs E2E testing with real multi-agent workflows.
- **Other messaging channels** — Discord, Zalo, Feishu/Lark, WhatsApp channel adapters are implemented but have not been tested end-to-end in production. Only Telegram has been validated with real users.
- **Skill system** — BM25 search, ZIP upload, SKILL.md parsing, and embedding hybrid search are implemented. Basic functionality verified but no full E2E flow testing with real agent usage.
- **Custom tools (runtime API)** — Shell-based custom tools with JSON Schema params, encrypted env vars, and HTTP CRUD are implemented. Not yet tested in a production workflow.
- **MCP integration** — stdio, SSE, and streamable-http transports with per-agent/per-user grants implemented. Not tested with real MCP servers in production.
- **Cron scheduling** — `at`, `every`, and cron expression scheduling implemented. Basic functionality works but no long-running production validation.
- **Text-to-Speech** — OpenAI, ElevenLabs, Edge, MiniMax providers implemented. Not tested end-to-end.
- **Docker sandbox** — Isolated code execution container support implemented. Not tested in production.
- **OpenTelemetry export** — OTLP gRPC/HTTP exporter implemented (build-tag gated). In-app tracing works; external OTel export not validated in production.
- **Tailscale integration** — tsnet listener implemented (build-tag gated). Not tested in a real deployment.
- **Browser pairing** — Pairing code flow implemented with CLI and web UI approval. Basic flow tested but not validated at scale.
- **HTTP API (`/v1/chat/completions`, `/v1/agents`, etc.)** — Endpoints implemented. Used by web dashboard but not tested for third-party consumer use cases.
- **Web dashboard (other pages)** — Skills, MCP, custom tools, cron, sessions, and config pages have basic rendering but UX not yet optimized for easy management and monitoring.

## Acknowledgments

GoClaw is built upon the original [OpenClaw](https://github.com/openclaw/openclaw) project. We are grateful for the architecture and vision that inspired this Go port.

## License

MIT
