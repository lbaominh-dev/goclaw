import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import i18n from "@/i18n";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import { toast } from "@/stores/use-toast-store";
import type { WorkerEndpointData, WorkerEndpointInput } from "@/types/agent";

export function useWorkerEndpoints() {
  const http = useHttp();
  const queryClient = useQueryClient();

  const { data: items = [], isLoading: loading, isFetching: fetching } = useQuery({
    queryKey: queryKeys.workerEndpoints.all,
    queryFn: async () => {
      const res = await http.get<{ items: WorkerEndpointData[] }>("/v1/worker-endpoints");
      return res.items ?? [];
    },
  });

  const refresh = useCallback(
    () => queryClient.invalidateQueries({ queryKey: queryKeys.workerEndpoints.all }),
    [queryClient],
  );

  const createWorkerEndpoint = useCallback(
    async (input: WorkerEndpointInput) => {
      try {
        const res = await http.post<WorkerEndpointData>("/v1/worker-endpoints", input);
        await refresh();
        toast.success(i18n.t("agents:workerEndpoints.toast.created"));
        return res;
      } catch (err) {
        toast.error(i18n.t("agents:workerEndpoints.toast.createFailed"), err instanceof Error ? err.message : "");
        throw err;
      }
    },
    [http, refresh],
  );

  const updateWorkerEndpoint = useCallback(
    async (id: string, input: Partial<WorkerEndpointInput>) => {
      try {
        await http.put(`/v1/worker-endpoints/${id}`, input);
        await refresh();
        toast.success(i18n.t("agents:workerEndpoints.toast.updated"));
      } catch (err) {
        toast.error(i18n.t("agents:workerEndpoints.toast.updateFailed"), err instanceof Error ? err.message : "");
        throw err;
      }
    },
    [http, refresh],
  );

  const deleteWorkerEndpoint = useCallback(
    async (id: string) => {
      try {
        await http.delete(`/v1/worker-endpoints/${id}`);
        await refresh();
        toast.success(i18n.t("agents:workerEndpoints.toast.deleted"));
      } catch (err) {
        toast.error(i18n.t("agents:workerEndpoints.toast.deleteFailed"), err instanceof Error ? err.message : "");
        throw err;
      }
    },
    [http, refresh],
  );

  return {
    items,
    loading,
    fetching,
    refresh,
    createWorkerEndpoint,
    updateWorkerEndpoint,
    deleteWorkerEndpoint,
  };
}
