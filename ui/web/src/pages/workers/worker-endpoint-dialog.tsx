import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { WorkerEndpointData, WorkerEndpointInput } from "@/types/agent";

interface WorkerEndpointDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  endpoint?: WorkerEndpointData | null;
  onSubmit: (input: WorkerEndpointInput) => Promise<unknown>;
}

export function WorkerEndpointDialog({ open, onOpenChange, endpoint, onSubmit }: WorkerEndpointDialogProps) {
  const { t } = useTranslation("agents");
  const { t: tc } = useTranslation("common");
  const [name, setName] = useState("");
  const [runtimeKind, setRuntimeKind] = useState("wails_desktop");
  const [endpointURL, setEndpointURL] = useState("");
  const [authToken, setAuthToken] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setName(endpoint?.name ?? "");
    setRuntimeKind(endpoint?.runtime_kind ?? "wails_desktop");
    setEndpointURL(endpoint?.endpoint_url ?? "");
    setAuthToken(endpoint?.auth_token ?? "");
    setError("");
  }, [endpoint, open]);

  const handleSubmit = async () => {
    if (!name.trim() || !runtimeKind.trim() || !endpointURL.trim() || !authToken.trim()) {
      setError(t("workerEndpoints.form.required"));
      return;
    }

    setLoading(true);
    setError("");
    try {
      await onSubmit({
        name: name.trim(),
        runtime_kind: runtimeKind.trim(),
        endpoint_url: endpointURL.trim(),
        auth_token: authToken.trim(),
      });
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("workerEndpoints.form.failed"));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => !loading && onOpenChange(next)}>
      <DialogContent className="max-h-[85vh] flex flex-col sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{endpoint ? t("workerEndpoints.form.editTitle") : t("workerEndpoints.form.createTitle")}</DialogTitle>
        </DialogHeader>

        <div className="grid gap-4 py-2">
          <div className="grid gap-1.5">
            <Label htmlFor="worker-endpoint-name">{t("workerEndpoints.form.name")}</Label>
            <Input id="worker-endpoint-name" value={name} onChange={(e) => setName(e.target.value)} className="text-base md:text-sm" />
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="worker-endpoint-runtime">{t("workerEndpoints.form.runtimeKind")}</Label>
            <Input id="worker-endpoint-runtime" value={runtimeKind} onChange={(e) => setRuntimeKind(e.target.value)} className="text-base md:text-sm" />
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="worker-endpoint-url">{t("workerEndpoints.form.endpointUrl")}</Label>
            <Input id="worker-endpoint-url" value={endpointURL} onChange={(e) => setEndpointURL(e.target.value)} className="text-base md:text-sm" />
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="worker-endpoint-token">{t("workerEndpoints.form.authToken")}</Label>
            <Input id="worker-endpoint-token" type="password" value={authToken} onChange={(e) => setAuthToken(e.target.value)} className="text-base md:text-sm" />
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={loading}>{tc("cancel")}</Button>
          <Button onClick={handleSubmit} disabled={loading}>{loading ? tc("saving") : endpoint ? tc("update") : tc("create")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
