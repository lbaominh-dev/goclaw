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
- `AUTH_HEADER` default `Authorization`
- `AUTH_TOKEN`
- `OPENCODE_COMMAND` default `opencode`
- `OPENCODE_ARGS_JSON` optional JSON string array
- `WORKSPACES_JSON` JSON object mapping workspace keys to absolute paths

## Commands

```bash
corepack pnpm install
corepack pnpm test
corepack pnpm build
```
