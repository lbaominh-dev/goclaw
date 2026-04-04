import { z } from "zod";
const EnvelopeSchema = z.object({
    type: z.string().trim().min(1),
    payload: z.unknown().optional(),
});
const DispatchPayloadSchema = z.object({
    jobId: z.string().trim().min(1),
    runtimeKind: z.literal("opencode"),
    job: z.unknown(),
    execution: z.object({
        workspaceKey: z.string().trim().min(1),
    }),
});
const CancelPayloadSchema = z.object({
    jobId: z.string().trim().min(1),
    reason: z.string().trim().optional(),
});
export function decodeEnvelope(raw) {
    return EnvelopeSchema.parse(JSON.parse(raw));
}
export function parseDispatchPayload(payload) {
    return DispatchPayloadSchema.parse(payload);
}
export function parseCancelPayload(payload) {
    return CancelPayloadSchema.parse(payload);
}
export function encodeEnvelope(message) {
    return JSON.stringify(message);
}
