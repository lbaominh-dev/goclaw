import { spawn } from "node:child_process";
export function createOpencodeRunner(config) {
    return async (payload, control) => new Promise((resolve) => {
        const child = spawn(config.opencodeCommand, config.opencodeArgs, {
            cwd: payload.workspacePath,
            stdio: ["pipe", "pipe", "pipe"],
            env: process.env,
        });
        let terminalSent = false;
        const resolveOnce = (message) => {
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
        child.stdout.on("data", (chunk) => {
            control.send({
                type: "job.output",
                jobId: payload.jobId,
                payload: { stream: "stdout", chunk: String(chunk) },
            });
        });
        child.stderr.on("data", (chunk) => {
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
        child.stdin.end(`${JSON.stringify(payload)}\n`);
    });
}
