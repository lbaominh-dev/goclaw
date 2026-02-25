import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";

interface WorkspaceSectionProps {
  workspace: string;
  restrictToWorkspace: boolean;
  onRestrictChange: (v: boolean) => void;
}

export function WorkspaceSection({
  workspace,
  restrictToWorkspace,
  onRestrictChange,
}: WorkspaceSectionProps) {
  return (
    <section className="space-y-4">
      <h3 className="text-sm font-medium text-muted-foreground">Workspace</h3>
      <div className="space-y-4 rounded-lg border p-4">
        <div className="space-y-2">
          <Label>Workspace Path</Label>
          <p className="rounded-md border bg-muted/50 px-3 py-2 font-mono text-sm text-muted-foreground">
            {workspace || "No workspace configured"}
          </p>
          <p className="text-xs text-muted-foreground">
            Automatically assigned when the agent is created. Per-user
            subdirectories are created at runtime.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Switch
            checked={restrictToWorkspace}
            onCheckedChange={onRestrictChange}
          />
          <div>
            <Label>Restrict to Workspace</Label>
            <p className="text-xs text-muted-foreground">
              Confine file access strictly within the workspace path.
            </p>
          </div>
        </div>
      </div>
    </section>
  );
}
