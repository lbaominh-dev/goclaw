import { mkdtempSync, mkdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { chmodSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import WebSocket from "ws";
import { afterEach, describe, expect, test } from "vitest";

import { createServer } from "./server.js";

const tempDirs: string[] = [];
const fixtureDir = join(dirname(fileURLToPath(import.meta.url)), "test", "fixtures");

afterEach(() => {
  for (const dir of tempDirs.splice(0)) {
    rmSync(dir, { recursive: true, force: true });
  }
});

function makeTempDir(): string {
  const dir = mkdtempSync(join(tmpdir(), "opencode-worker-server-"));
  tempDirs.push(dir);
  return dir;
}

function waitForOpen(socket: WebSocket): Promise<void> {
  return new Promise((resolve, reject) => {
    socket.once("open", () => resolve());
    socket.once("error", reject);
  });
}

function waitForClose(socket: WebSocket): Promise<number> {
  return new Promise((resolve) => {
    socket.once("close", (code) => resolve(code));
  });
}

function waitForHandshakeFailure(socket: WebSocket): Promise<string> {
  return new Promise((resolve) => {
    socket.once("error", (error) => resolve(error instanceof Error ? error.message : String(error)));
  });
}

function waitForMessageTypes(socket: WebSocket, expectedTypes: string[]): Promise<Array<Record<string, unknown>>> {
  return new Promise((resolve, reject) => {
    const messages: Array<Record<string, unknown>> = [];
    socket.on("message", (data) => {
      messages.push(JSON.parse(String(data)) as Record<string, unknown>);
      const types = messages.map((message) => String(message.type));
      const allPresent = expectedTypes.every((expectedType) => types.includes(expectedType));
      if (allPresent) {
        resolve(messages);
      }
    });
    socket.once("error", reject);
  });
}

describe("createServer", () => {
  test("rejects handshake with invalid auth header", async () => {
    const root = makeTempDir();
    const workspace = join(root, "workspace");
    mkdirSync(workspace);

    const server = createServer({
      port: 0,
      path: "/ws",
      authToken: "secret-token",
      authHeader: "Authorization",
      opencodeCommand: process.execPath,
      opencodeArgs: [],
      workspaces: { main: workspace },
    });
    await server.start();

    try {
      const socket = new WebSocket(server.url, {
        headers: { Authorization: "Bearer wrong" },
      });

      const message = await waitForHandshakeFailure(socket);
      expect(message).toMatch(/401/i);
    } finally {
      await server.stop();
    }
  });

  test("dispatches and cancels a running job over websocket", async () => {
    const root = makeTempDir();
    const workspace = join(root, "workspace");
    mkdirSync(workspace);
    const fakeOpencode = join(fixtureDir, "fake-opencode.js");
    chmodSync(fakeOpencode, 0o755);

    const server = createServer({
      port: 0,
      path: "/ws",
      authToken: "secret-token",
      authHeader: "Authorization",
      opencodeCommand: fakeOpencode,
      opencodeArgs: [],
      workspaces: { main: workspace },
    });
    await server.start();

    try {
      const socket = new WebSocket(server.url, {
        headers: { Authorization: "secret-token" },
      });
      await waitForOpen(socket);

      const messagesPromise = waitForMessageTypes(socket, ["job.started", "job.output", "job.failed"]);
      socket.send(
        JSON.stringify({
          type: "job.dispatch",
          payload: {
            jobId: "job-1",
            runtimeKind: "opencode",
            job: { message: "run", holdOpen: true },
            execution: { workspaceKey: "main" },
          },
        }),
      );

      await new Promise((resolve) => setTimeout(resolve, 100));
      socket.send(
        JSON.stringify({
          type: "job.cancel",
          payload: {
            jobId: "job-1",
            reason: "stop",
          },
        }),
      );

      const messages = await messagesPromise;
      expect(messages.some((message) => message.type === "job.started")).toBe(true);
      expect(messages.some((message) => message.type === "job.output")).toBe(true);
      expect(messages.at(-1)).toEqual({
        type: "job.failed",
        jobId: "job-1",
        error: { code: "CANCELED", message: "Job canceled by caller" },
      });

      socket.close();
      await waitForClose(socket);
    } finally {
      await server.stop();
    }
  });
});
