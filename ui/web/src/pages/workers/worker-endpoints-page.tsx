import { useState } from "react";
import { Link2, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { EmptyState } from "@/components/shared/empty-state";
import { PageHeader } from "@/components/shared/page-header";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { useDeferredLoading } from "@/hooks/use-deferred-loading";
import { useMinLoading } from "@/hooks/use-min-loading";
import { useWorkerEndpoints } from "@/hooks/use-worker-endpoints";
import type { WorkerEndpointData, WorkerEndpointInput } from "@/types/agent";
import { WorkerEndpointDialog } from "./worker-endpoint-dialog";

export function WorkerEndpointsPage() {
  const { t } = useTranslation("agents");
  const { t: tc } = useTranslation("common");
  const {
    items,
    loading,
    refresh,
    createWorkerEndpoint,
    updateWorkerEndpoint,
    deleteWorkerEndpoint,
  } = useWorkerEndpoints();
  const [formOpen, setFormOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<WorkerEndpointData | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<WorkerEndpointData | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

  const spinning = useMinLoading(loading);
  const showSkeleton = useDeferredLoading(loading && items.length === 0);

  const handleSubmit = async (input: WorkerEndpointInput) => {
    if (editTarget) {
      await updateWorkerEndpoint(editTarget.id, input);
      return;
    }
    await createWorkerEndpoint(input);
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleteLoading(true);
    try {
      await deleteWorkerEndpoint(deleteTarget.id);
      setDeleteTarget(null);
    } finally {
      setDeleteLoading(false);
    }
  };

  return (
    <div className="p-4 sm:p-6 pb-10">
      <PageHeader
        title={t("workerEndpoints.title")}
        description={t("workerEndpoints.description")}
        actions={
          <div className="flex gap-2">
            <Button size="sm" onClick={() => { setEditTarget(null); setFormOpen(true); }} className="gap-1">
              <Plus className="h-3.5 w-3.5" /> {t("workerEndpoints.add")}
            </Button>
            <Button variant="outline" size="sm" onClick={refresh} disabled={spinning} className="gap-1">
              <RefreshCw className={"h-3.5 w-3.5" + (spinning ? " animate-spin" : "")} /> {tc("refresh")}
            </Button>
          </div>
        }
      />

      <div className="mt-4">
        {showSkeleton ? (
          <TableSkeleton rows={4} />
        ) : items.length === 0 ? (
          <EmptyState icon={Link2} title={t("workerEndpoints.emptyTitle")} description={t("workerEndpoints.emptyDescription")} />
        ) : (
          <div className="overflow-x-auto rounded-md border">
            <table className="w-full min-w-[600px] text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="px-4 py-3 text-left font-medium">{t("workerEndpoints.columns.name")}</th>
                  <th className="px-4 py-3 text-left font-medium">{t("workerEndpoints.columns.runtime")}</th>
                  <th className="px-4 py-3 text-left font-medium">{t("workerEndpoints.columns.url")}</th>
                  <th className="px-4 py-3 text-left font-medium">{t("workerEndpoints.columns.token")}</th>
                  <th className="px-4 py-3 text-right font-medium">{tc("actions")}</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.id} className="border-b last:border-0 hover:bg-muted/30">
                    <td className="px-4 py-3 font-medium">{item.name}</td>
                    <td className="px-4 py-3"><Badge variant="outline">{item.runtime_kind}</Badge></td>
                    <td className="px-4 py-3 text-muted-foreground">{item.endpoint_url}</td>
                    <td className="px-4 py-3 text-muted-foreground">{t("workerEndpoints.tokenMasked")}</td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button variant="ghost" size="sm" onClick={() => { setEditTarget(item); setFormOpen(true); }} className="gap-1">
                          <Pencil className="h-3.5 w-3.5" /> {tc("edit")}
                        </Button>
                        <Button variant="ghost" size="sm" onClick={() => setDeleteTarget(item)} className="gap-1 text-destructive hover:text-destructive">
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <WorkerEndpointDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        endpoint={editTarget}
        onSubmit={handleSubmit}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t("workerEndpoints.delete.title")}
        description={t("workerEndpoints.delete.description", { name: deleteTarget?.name })}
        confirmLabel={t("workerEndpoints.delete.confirm")}
        variant="destructive"
        onConfirm={handleDelete}
        loading={deleteLoading}
      />
    </div>
  );
}
