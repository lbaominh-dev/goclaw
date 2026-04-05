import { spawn, type ChildProcess } from "node:child_process";

import type { WorkerConfig } from "./config.js";
import type { RunJob } from "./job-manager.js";
import type { WorkerReply } from "./protocol.js";

type RunnerConfig = Pick<WorkerConfig, "opencodeCommand" | "opencodeArgs">;

type RunnerPayload = {
  jobId: string;
  runtimeKind: "opencode";
  job: unknown;
  execution: { workspaceKey: string };
  workspacePath: string;
};

export function buildOpencodePrompt(payload: RunnerPayload): string {
  const job = (payload.job ?? {}) as Record<string, unknown>;
  const message = typeof job.message === "string" ? job.message.trim() : "";
  if (message !== "") {
    return message;
  }

  const lines = [
    "Execute this GoClaw local worker task.",
    `jobId: ${payload.jobId}`,
    `workspaceKey: ${payload.execution.workspaceKey}`,
  ];

  for (const key of ["runId", "sessionKey", "agentKey", "teamTaskId"] as const) {
    const value = job[key];
    if (typeof value === "string" && value.trim() !== "") {
      lines.push(`${key}: ${value}`);
    }
  }

  return lines.join("\n");
}

export function buildOpencodeCommand(config: RunnerConfig, payload: RunnerPayload): { command: string; args: string[] } {
  return {
    command: config.opencodeCommand,
    args: ["run", ...config.opencodeArgs, buildOpencodePrompt(payload)],
  };
}

type SpawnProcess = (command: string, args: string[], options: Parameters<typeof spawn>[2]) => ChildProcess;

export function createOpencodeRunner(
  config: Pick<WorkerConfig, "opencodeCommand" | "opencodeArgs">,
  spawnProcess: SpawnProcess = spawn,
): RunJob {
  return async (payload, control) =>
    new Promise<WorkerReply>((resolve) => {
      const command = buildOpencodeCommand(config, payload as RunnerPayload);
      const child = spawnProcess(command.command, command.args, {
        cwd: payload.workspacePath,
        stdio: ["ignore", "pipe", "pipe"],
        env: process.env,
      });

      let terminalSent = false;

      const resolveOnce = (message: WorkerReply) => {
        if (terminalSent) {
          return;
        }
        terminalSent = true;
        resolve(message);
      };

      control.onCancel(() => {
        if (child.exitCode === null) {
          child.kill("SIGTERM");
        }
      });

      child.once("spawn", () => {
        control.send({ type: "job.started", jobId: payload.jobId });
      });

      child.once("error", (error) => {
        resolveOnce({
          type: "job.failed",
          jobId: payload.jobId,
          error: { message: error.message },
        });
      });

      child.stdout?.on("data", (chunk: Buffer | string) => {
        control.send({
          type: "job.output",
          jobId: payload.jobId,
          payload: { stream: "stdout", chunk: String(chunk) },
        });
      });

      child.stderr?.on("data", (chunk: Buffer | string) => {
        control.send({
          type: "job.output",
          jobId: payload.jobId,
          payload: { stream: "stderr", chunk: String(chunk) },
        });
      });

      child.once("exit", (code, signal) => {
        if (control.isCanceled()) {
          resolveOnce({
            type: "job.failed",
            jobId: payload.jobId,
            error: { code: "CANCELED", message: "Job canceled by caller" },
          });
          return;
        }
        if (code === 0) {
          resolveOnce({
            type: "job.completed",
            jobId: payload.jobId,
            payload: { exitCode: 0 },
          });
          return;
        }
        resolveOnce({
          type: "job.failed",
          jobId: payload.jobId,
          error: {
            message: `opencode exited with code ${code ?? "null"}${signal ? ` signal ${signal}` : ""}`,
          },
        });
      });

    });
}
