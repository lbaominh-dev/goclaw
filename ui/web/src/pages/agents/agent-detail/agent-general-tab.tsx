import { useState, useMemo } from "react";
import { Save, Copy, Check, AlertCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Combobox } from "@/components/ui/combobox";
import { useProviders } from "@/pages/providers/hooks/use-providers";
import { useProviderModels } from "@/pages/providers/hooks/use-provider-models";
import type { AgentData } from "@/types/agent";

interface AgentGeneralTabProps {
  agent: AgentData;
  onUpdate: (updates: Record<string, unknown>) => Promise<void>;
}

export function AgentGeneralTab({ agent, onUpdate }: AgentGeneralTabProps) {
  const { providers } = useProviders();
  const enabledProviders = providers.filter((p) => p.enabled);

  const [displayName, setDisplayName] = useState(agent.display_name ?? "");
  const [provider, setProvider] = useState(agent.provider);
  const [model, setModel] = useState(agent.model);

  const selectedProviderId = useMemo(
    () => enabledProviders.find((p) => p.name === provider)?.id,
    [enabledProviders, provider],
  );
  const { models, loading: modelsLoading } = useProviderModels(selectedProviderId);
  const [contextWindow, setContextWindow] = useState(agent.context_window);
  const [maxToolIterations, setMaxToolIterations] = useState(agent.max_tool_iterations);
  const [workspace, setWorkspace] = useState(agent.workspace);
  const [restrictToWorkspace, setRestrictToWorkspace] = useState(agent.restrict_to_workspace);
  const [status, setStatus] = useState(agent.status);
  const [isDefault, setIsDefault] = useState(agent.is_default);
  const [saving, setSaving] = useState(false);
  const [copied, setCopied] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const handleSave = async () => {
    setSaving(true);
    setSaveError(null);
    setSaved(false);
    try {
      await onUpdate({
        display_name: displayName,
        provider,
        model,
        context_window: contextWindow,
        max_tool_iterations: maxToolIterations,
        workspace,
        restrict_to_workspace: restrictToWorkspace,
        status,
        is_default: isDefault,
      });
      setSaved(true);
      setTimeout(() => setSaved(false), 3000);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  };

  const copyAgentKey = async () => {
    await navigator.clipboard.writeText(agent.agent_key);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="max-w-2xl space-y-6">
      {/* Identity */}
      <section className="space-y-4">
        <h3 className="text-sm font-medium text-muted-foreground">Identity</h3>
        <div className="space-y-4 rounded-lg border p-4">
          <div className="space-y-2">
            <Label>Agent Key</Label>
            <div className="flex items-center gap-2">
              <Input value={agent.agent_key} disabled className="font-mono text-sm" />
              <Button
                variant="ghost"
                size="icon"
                className="shrink-0"
                onClick={copyAgentKey}
              >
                {copied ? <Check className="h-3.5 w-3.5 text-green-500" /> : <Copy className="h-3.5 w-3.5" />}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">Unique identifier, cannot be changed.</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="displayName">Display Name</Label>
            <Input
              id="displayName"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="e.g. My Assistant"
            />
            <p className="text-xs text-muted-foreground">
              Friendly name shown in the UI. Leave empty to use the agent key.
            </p>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>Status</Label>
              <Select value={status} onValueChange={setStatus}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="active">Active</SelectItem>
                  <SelectItem value="inactive">Inactive</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-end pb-2">
              <div className="flex items-center gap-2">
                <Switch checked={isDefault} onCheckedChange={setIsDefault} />
                <Label>Default Agent</Label>
              </div>
            </div>
          </div>
        </div>
      </section>

      <Separator />

      {/* LLM Configuration */}
      <section className="space-y-4">
        <h3 className="text-sm font-medium text-muted-foreground">LLM Configuration</h3>
        <div className="space-y-4 rounded-lg border p-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>Provider</Label>
              {enabledProviders.length > 0 ? (
                <Select value={provider} onValueChange={(v) => { setProvider(v); setModel(""); }}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select provider" />
                  </SelectTrigger>
                  <SelectContent>
                    {enabledProviders.map((p) => (
                      <SelectItem key={p.name} value={p.name}>
                        {p.display_name || p.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  value={provider}
                  onChange={(e) => setProvider(e.target.value)}
                  placeholder="openrouter"
                />
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="model">Model</Label>
              <Combobox
                value={model}
                onChange={setModel}
                options={models.map((m) => ({ value: m.id, label: m.name }))}
                placeholder={modelsLoading ? "Loading models..." : "Enter or select model"}
              />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="contextWindow">Context Window</Label>
              <Input
                id="contextWindow"
                type="number"
                value={contextWindow || ""}
                onChange={(e) => setContextWindow(Number(e.target.value) || 0)}
                placeholder="200000"
              />
              <p className="text-xs text-muted-foreground">Token limit for the model context.</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="maxToolIterations">Max Tool Iterations</Label>
              <Input
                id="maxToolIterations"
                type="number"
                value={maxToolIterations || ""}
                onChange={(e) => setMaxToolIterations(Number(e.target.value) || 0)}
                placeholder="25"
              />
              <p className="text-xs text-muted-foreground">Max tool calls per request.</p>
            </div>
          </div>
        </div>
      </section>

      <Separator />

      {/* Workspace */}
      <section className="space-y-4">
        <h3 className="text-sm font-medium text-muted-foreground">Workspace</h3>
        <div className="space-y-4 rounded-lg border p-4">
          <div className="space-y-2">
            <Label htmlFor="workspace">Workspace Path</Label>
            <Input
              id="workspace"
              value={workspace}
              onChange={(e) => setWorkspace(e.target.value)}
              placeholder="/path/to/workspace"
              className="font-mono text-sm"
            />
            <p className="text-xs text-muted-foreground">
              Filesystem root the agent can access. Leave empty for no filesystem access.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Switch checked={restrictToWorkspace} onCheckedChange={setRestrictToWorkspace} />
            <div>
              <Label>Restrict to Workspace</Label>
              <p className="text-xs text-muted-foreground">
                Confine file access strictly within the workspace path.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* Save */}
      {saveError && (
        <div className="flex items-center gap-2 rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 shrink-0" />
          {saveError}
        </div>
      )}
      <div className="flex items-center justify-end gap-2">
        {saved && (
          <span className="flex items-center gap-1 text-sm text-green-600">
            <Check className="h-3.5 w-3.5" /> Saved
          </span>
        )}
        <Button onClick={handleSave} disabled={saving} className="gap-1.5">
          <Save className="h-4 w-4" />
          {saving ? "Saving..." : "Save Changes"}
        </Button>
      </div>
    </div>
  );
}
