import type { DispatchPayload, WorkerReply } from "./protocol.js";

export type SendMessage = (message: WorkerReply) => void;

export type RunControl = {
  send: SendMessage;
  onCancel(handler: () => void): void;
  isCanceled(): boolean;
};

export type RunJob = (
  payload: DispatchPayload & { workspacePath: string },
  control: RunControl,
) => Promise<WorkerReply>;

type JobManagerOptions = {
  workspaces: Record<string, string>;
  runJob: RunJob;
  send: SendMessage;
};

type ActiveJob = {
  cancelHandlers: Array<() => void>;
  canceled: boolean;
  terminalSent: boolean;
};

export class JobManager {
  private readonly workspaces: Record<string, string>;
  private readonly runJob: RunJob;
  private readonly sendMessage: SendMessage;
  private readonly activeJobs = new Map<string, ActiveJob>();

  constructor(options: JobManagerOptions) {
    this.workspaces = options.workspaces;
    this.runJob = options.runJob;
    this.sendMessage = options.send;
  }

  hasActiveJob(jobId: string): boolean {
    return this.activeJobs.has(jobId);
  }

  async dispatch(payload: DispatchPayload): Promise<void> {
    const workspacePath = this.workspaces[payload.execution.workspaceKey];
    if (!workspacePath) {
      throw new Error(`unknown workspaceKey: ${payload.execution.workspaceKey}`);
    }
    if (this.activeJobs.has(payload.jobId)) {
      throw new Error(`duplicate active jobId: ${payload.jobId}`);
    }

    const activeJob: ActiveJob = {
      cancelHandlers: [],
      canceled: false,
      terminalSent: false,
    };
    this.activeJobs.set(payload.jobId, activeJob);

    try {
      const terminal = await this.runJob({ ...payload, workspacePath }, {
        send: (message) => this.sendMessage(message),
        onCancel: (handler) => {
          activeJob.cancelHandlers.push(handler);
        },
        isCanceled: () => activeJob.canceled,
      });
      if (!activeJob.terminalSent) {
        activeJob.terminalSent = true;
        this.sendMessage(terminal);
      }
    } finally {
      this.activeJobs.delete(payload.jobId);
    }
  }

  async cancel(payload: { jobId: string; reason?: string }): Promise<void> {
    const activeJob = this.activeJobs.get(payload.jobId);
    if (!activeJob || activeJob.canceled) {
      return;
    }
    activeJob.canceled = true;
    for (const handler of activeJob.cancelHandlers) {
      handler();
    }
  }

  async cancelAll(): Promise<void> {
    await Promise.all(
      Array.from(this.activeJobs.keys(), (jobId) => this.cancel({ jobId, reason: "socket disconnected" })),
    );
  }
}
