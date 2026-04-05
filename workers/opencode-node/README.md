# opencode-node worker

Standalone local worker server for GoClaw outbound worker connections.

## Behavior

- validates a configured auth header during WebSocket upgrade
- accepts `job.dispatch` and `job.cancel`
- supports only `runtimeKind = opencode`
- resolves `payload.execution.workspaceKey` to a configured absolute path
- spawns a fresh local process per job and streams stdout/stderr as `job.output`
- emits `job.started`, then exactly one terminal `job.completed` or `job.failed`

## Config

Environment variables:

- `PORT`
- `WS_PATH` default `/ws`
- `AUTH_HEADER` default `Authorization`
- `AUTH_TOKEN`
- `OPENCODE_COMMAND` default `opencode`
- `OPENCODE_ARGS_JSON` optional JSON string array
- `WORKSPACES_JSON` JSON object mapping workspace keys to absolute paths

For `runtimeKind = opencode`, the worker invokes the OpenCode CLI as:

```bash
opencode run "<task prompt>"
```

`OPENCODE_COMMAND` should point to the `opencode` executable.

`OPENCODE_ARGS_JSON` should contain extra flags for `opencode run`, not the `run` subcommand itself. Example:

```dotenv
OPENCODE_COMMAND=opencode
OPENCODE_ARGS_JSON='["--model","anthropic/claude-sonnet-4"]'
```

On startup, the worker also loads dotenv files from its current working directory:

1. `.env` loads first.
2. `.env.local` loads second and can override values loaded from `.env`.
3. Explicit shell or runtime environment values still win over both dotenv files.

Example `.env`:

```dotenv
PORT=8111
WS_PATH=worker
AUTH_TOKEN=replace-me
OPENCODE_COMMAND=opencode
WORKSPACES_JSON={"main":"/absolute/path/to/workspace"}
```

## Commands

```bash
corepack pnpm install
corepack pnpm test
corepack pnpm build
```
