import { mkdtempSync, mkdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, test } from "vitest";

import { loadConfig } from "./config.js";

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
});
