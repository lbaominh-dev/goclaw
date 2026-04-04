import { isAbsolute, resolve } from "node:path";

export type WorkerConfigInput = {
  port?: number;
  path?: string;
  authHeader?: string;
  authToken?: string;
  opencodeCommand?: string;
  opencodeArgs?: string[];
  workspaces?: Record<string, string>;
};

export type WorkerConfig = {
  port: number;
  path: string;
  authHeader: string;
  authToken: string;
  opencodeCommand: string;
  opencodeArgs: string[];
  workspaces: Record<string, string>;
};

type EnvMap = Record<string, string | undefined>;

export function loadConfig(input?: WorkerConfigInput, env: EnvMap = process.env): WorkerConfig {
  const port = input?.port ?? parseInteger(env.PORT) ?? 8787;
  const path = normalizePath(input?.path ?? env.WS_PATH ?? "/ws");
  const authHeader = (input?.authHeader ?? env.AUTH_HEADER ?? "Authorization").trim();
  const authToken = (input?.authToken ?? env.AUTH_TOKEN ?? "").trim();
  const opencodeCommand = (input?.opencodeCommand ?? env.OPENCODE_COMMAND ?? "opencode").trim();
  const opencodeArgs = input?.opencodeArgs ?? parseStringArray(env.OPENCODE_ARGS_JSON) ?? [];
  const workspaces = normalizeWorkspaces(input?.workspaces ?? parseWorkspaceMap(env.WORKSPACES_JSON) ?? {});

  if (!Number.isInteger(port) || port < 0 || port > 65535) {
    throw new Error("port must be a valid TCP port");
  }
  if (authHeader === "") {
    throw new Error("authHeader is required");
  }
  if (authToken === "") {
    throw new Error("authToken is required");
  }
  if (opencodeCommand === "") {
    throw new Error("opencodeCommand is required");
  }
  if (Object.keys(workspaces).length === 0) {
    throw new Error("at least one workspace is required");
  }

  return {
    port,
    path,
    authHeader,
    authToken,
    opencodeCommand,
    opencodeArgs,
    workspaces,
  };
}

function normalizePath(value: string): string {
  const trimmed = value.trim();
  if (trimmed === "") {
    throw new Error("path is required");
  }
  if (trimmed.startsWith("/")) {
    return trimmed;
  }
  return `/${trimmed}`;
}

function parseInteger(value: string | undefined): number | undefined {
  if (!value || value.trim() === "") {
    return undefined;
  }
  return Number.parseInt(value, 10);
}

function parseStringArray(value: string | undefined): string[] | undefined {
  if (!value || value.trim() === "") {
    return undefined;
  }
  const parsed = JSON.parse(value) as unknown;
  if (!Array.isArray(parsed) || parsed.some((item) => typeof item !== "string")) {
    throw new Error("OPENCODE_ARGS_JSON must be a JSON string array");
  }
  return parsed;
}

function parseWorkspaceMap(value: string | undefined): Record<string, string> | undefined {
  if (!value || value.trim() === "") {
    return undefined;
  }
  const parsed = JSON.parse(value) as unknown;
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== "object") {
    throw new Error("WORKSPACES_JSON must be a JSON object");
  }
  return parsed as Record<string, string>;
}

function normalizeWorkspaces(input: Record<string, string>): Record<string, string> {
  const normalized: Record<string, string> = {};
  for (const [key, value] of Object.entries(input)) {
    const workspaceKey = key.trim();
    const workspacePath = String(value).trim();
    if (workspaceKey === "") {
      throw new Error("workspace keys must be non-empty");
    }
    if (!isAbsolute(workspacePath)) {
      throw new Error(`workspace path for ${workspaceKey} must be absolute`);
    }
    normalized[workspaceKey] = resolve(workspacePath);
  }
  return normalized;
}
