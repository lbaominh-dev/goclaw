import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import type { AgentExecutionMode } from "@/types/agent";

interface ExecutionSectionProps {
  executionMode: AgentExecutionMode;
  onExecutionModeChange: (value: AgentExecutionMode) => void;
  localRuntimeKind: string;
  onLocalRuntimeKindChange: (value: string) => void;
  boundWorkerId: string;
  onBoundWorkerIdChange: (value: string) => void;
}

export function ExecutionSection({
  executionMode,
  onExecutionModeChange,
  localRuntimeKind,
  onLocalRuntimeKindChange,
  boundWorkerId,
  onBoundWorkerIdChange,
}: ExecutionSectionProps) {
  const { t } = useTranslation("agents");

  return (
    <section className="space-y-3 rounded-lg border p-3 sm:p-4 overflow-hidden">
      <h3 className="text-sm font-medium">{t("detail.execution")}</h3>

      <div className="space-y-1.5">
        <Label className="text-xs">{t("execution.modeLabel")}</Label>
        <Select value={executionMode} onValueChange={(value) => onExecutionModeChange(value as AgentExecutionMode)}>
          <SelectTrigger className="w-full text-base md:text-sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="server">{t("execution.modeServer")}</SelectItem>
            <SelectItem value="local_worker">{t("execution.modeLocalWorker")}</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">{t("execution.modeHint")}</p>
      </div>

      {executionMode === "local_worker" && (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="localRuntimeKind" className="text-xs">{t("execution.runtimeKindLabel")}</Label>
            <Input
              id="localRuntimeKind"
              value={localRuntimeKind}
              onChange={(e) => onLocalRuntimeKindChange(e.target.value)}
              placeholder={t("execution.runtimeKindPlaceholder")}
              className="text-base md:text-sm"
            />
            <p className="text-xs text-muted-foreground">{t("execution.runtimeKindHint")}</p>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="boundWorkerId" className="text-xs">{t("execution.boundWorkerIdLabel")}</Label>
            <Input
              id="boundWorkerId"
              value={boundWorkerId}
              onChange={(e) => onBoundWorkerIdChange(e.target.value)}
              placeholder={t("execution.boundWorkerIdPlaceholder")}
              className="text-base md:text-sm"
            />
            <p className="text-xs text-muted-foreground">{t("execution.boundWorkerIdHint")}</p>
          </div>
        </div>
      )}
    </section>
  );
}
