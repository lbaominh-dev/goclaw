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

export type Envelope = z.infer<typeof EnvelopeSchema>;
export type DispatchPayload = z.infer<typeof DispatchPayloadSchema>;
export type CancelPayload = z.infer<typeof CancelPayloadSchema>;

export type WorkerReply =
  | { type: "job.started"; jobId: string }
  | { type: "job.output"; jobId: string; payload: { stream: "stdout" | "stderr"; chunk: string } }
  | { type: "job.completed"; jobId: string; payload: unknown }
  | { type: "job.failed"; jobId: string; error: { code?: string; message: string } };

export function decodeEnvelope(raw: string): Envelope {
  return EnvelopeSchema.parse(JSON.parse(raw) as unknown);
}

export function parseDispatchPayload(payload: unknown): DispatchPayload {
  return DispatchPayloadSchema.parse(payload);
}

export function parseCancelPayload(payload: unknown): CancelPayload {
  return CancelPayloadSchema.parse(payload);
}

export function encodeEnvelope(message: WorkerReply): string {
  return JSON.stringify(message);
}
