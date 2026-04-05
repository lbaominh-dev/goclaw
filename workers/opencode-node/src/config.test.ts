import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, test } from "vitest";

import { loadConfig } from "./config.js";
import { loadStartupEnv } from "./env-loader.js";

const tempDirs: string[] = [];

afterEach(() => {
  for (const dir of tempDirs.splice(0)) {
    rmSync(dir, { recursive: true, force: true });
  }
});

function makeTempDir(): string {
  const dir = mkdtempSync(join(tmpdir(), "opencode-worker-config-"));
  tempDirs.push(dir);
  return dir;
}

function restoreEnv(name: string, value: string | undefined): void {
  if (value === undefined) {
    delete process.env[name];
    return;
  }
  process.env[name] = value;
}

describe("loadConfig", () => {
  test("normalizes workspace paths and defaults auth header", () => {
    const root = makeTempDir();
    const workspace = join(root, "workspace-a");
    mkdirSync(workspace);

    const config = loadConfig({
      port: 8787,
      authToken: "secret-token",
      opencodeCommand: process.execPath,
      workspaces: {
        main: workspace,
      },
    });

    expect(config.authHeader).toBe("Authorization");
    expect(config.path).toBe("/ws");
    expect(config.workspaces.main).toBe(workspace);
  });

  test("rejects non-absolute workspace paths", () => {
    expect(() =>
      loadConfig({
        port: 8787,
        authToken: "secret-token",
        workspaces: {
          main: "relative/path",
        },
      }),
    ).toThrow(/absolute/i);
  });

  test("reads values from environment variables", () => {
    const root = makeTempDir();
    const workspace = join(root, "workspace-b");
    mkdirSync(workspace);

    const config = loadConfig(undefined, {
      PORT: "8999",
      WS_PATH: "worker",
      AUTH_TOKEN: "env-secret",
      AUTH_HEADER: "X-Worker-Token",
      OPENCODE_COMMAND: process.execPath,
      WORKSPACES_JSON: JSON.stringify({ env: workspace }),
    });

    expect(config.port).toBe(8999);
    expect(config.path).toBe("/worker");
    expect(config.authHeader).toBe("X-Worker-Token");
    expect(config.authToken).toBe("env-secret");
    expect(config.workspaces.env).toBe(workspace);
  });

  test("loads startup values from .env and lets .env.local override them", () => {
    const root = makeTempDir();
    const workspace = join(root, "workspace-c");
    mkdirSync(workspace);
    writeFileSync(join(root, ".env"), "PORT=8111\nWS_PATH=from-env\n");
    writeFileSync(join(root, ".env.local"), "PORT=8222\n");

    const originalCwd = process.cwd();
    const originalPort = process.env.PORT;
    const originalWsPath = process.env.WS_PATH;
    delete process.env.PORT;
    delete process.env.WS_PATH;
    process.chdir(root);

    try {
      const env = loadStartupEnv({
        AUTH_TOKEN: "secret-token",
        OPENCODE_COMMAND: process.execPath,
        WORKSPACES_JSON: JSON.stringify({ main: workspace }),
      });

      const config = loadConfig(undefined, env);

      expect(env.PORT).toBe("8222");
      expect(env.WS_PATH).toBe("from-env");
      expect(config.port).toBe(8222);
      expect(config.path).toBe("/from-env");
    } finally {
      process.chdir(originalCwd);
      restoreEnv("PORT", originalPort);
      restoreEnv("WS_PATH", originalWsPath);
    }
  });

  test("keeps explicit env values over .env.local overrides", () => {
    const root = makeTempDir();
    writeFileSync(join(root, ".env"), "PORT=8111\n");
    writeFileSync(join(root, ".env.local"), "PORT=8222\n");

    const originalCwd = process.cwd();
    process.chdir(root);

    try {
      const env = loadStartupEnv({ PORT: "9001" });

      expect(env.PORT).toBe("9001");
    } finally {
      process.chdir(originalCwd);
    }
  });
});
