import { EventEmitter } from "node:events";

import { afterEach, describe, expect, test, vi } from "vitest";

import { buildOpencodeCommand, createOpencodeRunner } from "./opencode-runner.js";

afterEach(() => {
  vi.restoreAllMocks();
});

function createFakeChild() {
  const stdout = new EventEmitter();
  const stderr = new EventEmitter();
  const child = new EventEmitter() as EventEmitter & {
    stdout: EventEmitter;
    stderr: EventEmitter;
    kill: ReturnType<typeof vi.fn>;
    exitCode: number | null;
  };
  child.stdout = stdout;
  child.stderr = stderr;
  child.kill = vi.fn();
  child.exitCode = null;
  return child;
}

describe("buildOpencodeCommand", () => {
  test("builds opencode run arguments with the task message as the prompt", () => {
    const command = buildOpencodeCommand(
      {
        opencodeCommand: "opencode",
        opencodeArgs: ["--model", "anthropic/claude-sonnet-4"],
      },
      {
        jobId: "job-1",
        runtimeKind: "opencode",
        job: {
          message: "Refactor the worker runner",
          runId: "run-1",
          sessionKey: "session-1",
        },
        execution: { workspaceKey: "main" },
        workspacePath: "/tmp/main",
      },
    );

    expect(command.command).toBe("opencode");
    expect(command.args).toEqual(["run", "--model", "anthropic/claude-sonnet-4", "--session", "session-1", "Refactor the worker runner"]);
  });

  test("falls back to a structured prompt when message is missing", () => {
    const command = buildOpencodeCommand(
      {
        opencodeCommand: "opencode",
        opencodeArgs: [],
      },
      {
        jobId: "job-2",
        runtimeKind: "opencode",
        job: {
          runId: "run-2",
          sessionKey: "session-2",
          agentKey: "agent-7",
        },
        execution: { workspaceKey: "main" },
        workspacePath: "/tmp/main",
      },
    );

    expect(command.args[0]).toBe("run");
    expect(command.args.at(-1)).toContain("runId: run-2");
    expect(command.args.at(-1)).toContain("sessionKey: session-2");
    expect(command.args.at(-1)).toContain("agentKey: agent-7");
  });

  test("reuses the chat session key as the worker conversation identity", () => {
    const command = buildOpencodeCommand(
      {
        opencodeCommand: "opencode",
        opencodeArgs: [],
      },
      {
        jobId: "job-3",
        runtimeKind: "opencode",
        job: {
          message: "Continue",
          sessionKey: "agent:test:ws:direct:chat-1",
        },
        execution: { workspaceKey: "main" },
        workspacePath: "/tmp/main",
      },
    );

    expect(command.args).toContain("--session");
    expect(command.args).toContain("agent:test:ws:direct:chat-1");
  });

  test("streams output and completes only after child exit", async () => {
    const child = createFakeChild();
    const spawnProcess = vi.fn(() => child as never);

    const sent: unknown[] = [];
    const runner = createOpencodeRunner({ opencodeCommand: "opencode", opencodeArgs: [] }, spawnProcess);
    const runPromise = runner(
      {
        jobId: "job-1",
        runtimeKind: "opencode",
        job: { message: "Do the work" },
        execution: { workspaceKey: "main" },
        workspacePath: "/tmp/main",
      },
      {
        send: (message) => sent.push(message),
        onCancel: () => undefined,
        isCanceled: () => false,
      },
    );

    child.emit("spawn");
    child.stdout.emit("data", "hello\n");
    child.stderr.emit("data", "warn\n");
    child.emit("exit", 0, null);

    await expect(runPromise).resolves.toEqual({
      type: "job.completed",
      jobId: "job-1",
      payload: { exitCode: 0 },
    });
    expect(sent).toContainEqual({ type: "job.started", jobId: "job-1" });
    expect(sent).toContainEqual({
      type: "job.output",
      jobId: "job-1",
      payload: { type: "Thinking", stream: "stdout", chunk: "hello\n" },
    });
    expect(sent).toContainEqual({
      type: "job.output",
      jobId: "job-1",
      payload: { type: "Error", stream: "stderr", chunk: "warn\n" },
    });
  });

  test("kills the child on cancel and returns canceled failure", async () => {
    const child = createFakeChild();
    const spawnProcess = vi.fn(() => child as never);

    let cancelHandler = () => {};
    const runner = createOpencodeRunner({ opencodeCommand: "opencode", opencodeArgs: [] }, spawnProcess);
    const runPromise = runner(
      {
        jobId: "job-2",
        runtimeKind: "opencode",
        job: { message: "Do the work" },
        execution: { workspaceKey: "main" },
        workspacePath: "/tmp/main",
      },
      {
        send: () => undefined,
        onCancel: (handler) => {
          cancelHandler = handler;
        },
        isCanceled: () => true,
      },
    );

    cancelHandler();
    child.emit("exit", null, "SIGTERM");

    await expect(runPromise).resolves.toEqual({
      type: "job.failed",
      jobId: "job-2",
      error: { code: "CANCELED", message: "Job canceled by caller" },
    });
    expect(child.kill).toHaveBeenCalledWith("SIGTERM");
  });
});
