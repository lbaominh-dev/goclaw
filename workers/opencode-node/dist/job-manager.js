export class JobManager {
    workspaces;
    runJob;
    sendMessage;
    activeJobs = new Map();
    constructor(options) {
        this.workspaces = options.workspaces;
        this.runJob = options.runJob;
        this.sendMessage = options.send;
    }
    hasActiveJob(jobId) {
        return this.activeJobs.has(jobId);
    }
    async dispatch(payload) {
        const workspacePath = this.workspaces[payload.execution.workspaceKey];
        if (!workspacePath) {
            throw new Error(`unknown workspaceKey: ${payload.execution.workspaceKey}`);
        }
        if (this.activeJobs.has(payload.jobId)) {
            throw new Error(`duplicate active jobId: ${payload.jobId}`);
        }
        const activeJob = {
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
        }
        finally {
            this.activeJobs.delete(payload.jobId);
        }
    }
    async cancel(payload) {
        const activeJob = this.activeJobs.get(payload.jobId);
        if (!activeJob || activeJob.canceled) {
            return;
        }
        activeJob.canceled = true;
        for (const handler of activeJob.cancelHandlers) {
            handler();
        }
    }
    async cancelAll() {
        await Promise.all(Array.from(this.activeJobs.keys(), (jobId) => this.cancel({ jobId, reason: "socket disconnected" })));
    }
}
