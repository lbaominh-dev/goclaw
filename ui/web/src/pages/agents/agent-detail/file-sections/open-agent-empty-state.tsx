import { FileText, Info } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import type { BootstrapFile } from "@/types/agent";
import { FILE_DESCRIPTIONS } from "./file-utils";

interface OpenAgentEmptyStateProps {
  files: BootstrapFile[];
}

export function OpenAgentEmptyState({ files }: OpenAgentEmptyStateProps) {
  return (
    <div className="max-w-2xl space-y-4">
      <div className="flex items-start gap-3 rounded-lg border border-info/30 bg-sky-500/5 p-4">
        <Info className="mt-0.5 h-5 w-5 shrink-0 text-sky-600 dark:text-sky-400" />
        <div className="space-y-2 text-sm">
          <p className="font-medium">Open Agent - Per-User Context Files</p>
          <p className="text-muted-foreground">
            This is an <strong>open</strong> agent. Context files (AGENTS.md,
            SOUL.md, TOOLS.md, etc.) are personalized for each user. They are
            automatically created from templates when a user first chats with
            this agent.
          </p>
          <p className="text-muted-foreground">
            Agent-level files shown here are empty because open agents store all
            context per-user in the{" "}
            <code className="rounded bg-muted px-1 py-0.5 text-xs">
              user_context_files
            </code>{" "}
            table.
          </p>
        </div>
      </div>

      <div className="rounded-lg border p-4">
        <h4 className="mb-3 text-sm font-medium">Context Files</h4>
        <div className="space-y-2">
          {files.map((file) => (
            <div
              key={file.name}
              className="flex items-center gap-3 rounded-md bg-muted/50 px-3 py-2"
            >
              <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
              <div className="min-w-0 flex-1">
                <div className="text-sm font-medium">{file.name}</div>
                <div className="text-xs text-muted-foreground">
                  {FILE_DESCRIPTIONS[file.name] || "Context file"}
                </div>
              </div>
              <Badge variant="outline" className="shrink-0 text-[10px]">
                per-user
              </Badge>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
