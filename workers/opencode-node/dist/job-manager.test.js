import { describe, expect, test, vi } from "vitest";
import { JobManager } from "./job-manager.js";
describe("JobManager", () => {
    test("starts a job and removes it after completion", async () => {
        const send = vi.fn();
        const runner = vi.fn(async ({ jobId }) => {
            send({ type: "job.started", jobId });
            send({ type: "job.completed", jobId, payload: { ok: true } });
            return { type: "job.completed", jobId, payload: { ok: true } };
        });
        const manager = new JobManager({
            workspaces: { main: "/tmp/main" },
            runJob: runner,
            send,
        });
        await manager.dispatch({
            jobId: "job-1",
            runtimeKind: "opencode",
            job: { task: 1 },
            execution: { workspaceKey: "main" },
        });
        expect(runner).toHaveBeenCalledTimes(1);
        expect(manager.hasActiveJob("job-1")).toBe(false);
    });
    test("rejects duplicate active job ids", async () => {
        let release = () => { };
        const blocker = new Promise((resolve) => {
            release = resolve;
        });
        const manager = new JobManager({
            workspaces: { main: "/tmp/main" },
            runJob: async ({ jobId }, control) => {
                control.send({ type: "job.started", jobId });
                await blocker;
                return { type: "job.completed", jobId, payload: {} };
            },
            send: vi.fn(),
        });
        const first = manager.dispatch({
            jobId: "job-1",
            runtimeKind: "opencode",
            job: {},
            execution: { workspaceKey: "main" },
        });
        await expect(manager.dispatch({
            jobId: "job-1",
            runtimeKind: "opencode",
            job: {},
            execution: { workspaceKey: "main" },
        })).rejects.toThrow(/duplicate/i);
        release();
        await first;
    });
    test("cancel stops active job and emits canceled failure once", async () => {
        const send = vi.fn();
        let canceled = false;
        const manager = new JobManager({
            workspaces: { main: "/tmp/main" },
            runJob: async ({ jobId }, control) => {
                control.send({ type: "job.started", jobId });
                await new Promise((resolve) => {
                    control.onCancel(() => {
                        canceled = true;
                        resolve();
                    });
                });
                return {
                    type: "job.failed",
                    jobId,
                    error: { code: "CANCELED", message: "Job canceled by caller" },
                };
            },
            send,
        });
        const run = manager.dispatch({
            jobId: "job-2",
            runtimeKind: "opencode",
            job: {},
            execution: { workspaceKey: "main" },
        });
        await manager.cancel({ jobId: "job-2", reason: "stop" });
        await run;
        expect(canceled).toBe(true);
        expect(send).toHaveBeenCalledWith({
            type: "job.failed",
            jobId: "job-2",
            error: { code: "CANCELED", message: "Job canceled by caller" },
        });
        expect(send.mock.calls.filter((call) => call[0].type === "job.failed")).toHaveLength(1);
        expect(manager.hasActiveJob("job-2")).toBe(false);
    });
});
