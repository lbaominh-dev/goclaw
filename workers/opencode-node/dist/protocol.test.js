import { describe, expect, test } from "vitest";
import { decodeEnvelope, encodeEnvelope, parseCancelPayload, parseDispatchPayload, } from "./protocol.js";
describe("protocol", () => {
    test("parses valid dispatch payload", () => {
        const envelope = decodeEnvelope(JSON.stringify({
            type: "job.dispatch",
            payload: {
                jobId: "job-1",
                runtimeKind: "opencode",
                job: { message: "run" },
                execution: { workspaceKey: "main" },
            },
        }));
        expect(envelope.type).toBe("job.dispatch");
        expect(parseDispatchPayload(envelope.payload)).toMatchObject({
            jobId: "job-1",
            runtimeKind: "opencode",
            execution: { workspaceKey: "main" },
        });
    });
    test("rejects dispatch without workspace key", () => {
        const envelope = decodeEnvelope(JSON.stringify({
            type: "job.dispatch",
            payload: {
                jobId: "job-1",
                runtimeKind: "opencode",
                job: {},
                execution: {},
            },
        }));
        expect(() => parseDispatchPayload(envelope.payload)).toThrow(/workspaceKey/i);
    });
    test("parses valid cancel payload", () => {
        const envelope = decodeEnvelope(JSON.stringify({
            type: "job.cancel",
            payload: {
                jobId: "job-2",
                reason: "stop",
            },
        }));
        expect(parseCancelPayload(envelope.payload)).toEqual({
            jobId: "job-2",
            reason: "stop",
        });
    });
    test("encodes reply envelope", () => {
        expect(JSON.parse(encodeEnvelope({
            type: "job.started",
            jobId: "job-3",
        }))).toEqual({
            type: "job.started",
            jobId: "job-3",
        });
    });
});
