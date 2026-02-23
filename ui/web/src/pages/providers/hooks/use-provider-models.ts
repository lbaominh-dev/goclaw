import { useState, useEffect, useCallback } from "react";
import { useHttp } from "@/hooks/use-ws";

export interface ModelInfo {
  id: string;
  name?: string;
}

export function useProviderModels(providerId: string | undefined) {
  const http = useHttp();
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [loading, setLoading] = useState(false);

  const load = useCallback(
    async (id: string) => {
      setLoading(true);
      try {
        const res = await http.get<{ models: ModelInfo[] }>(
          `/v1/providers/${id}/models`,
        );
        setModels(res.models ?? []);
      } catch {
        setModels([]);
      } finally {
        setLoading(false);
      }
    },
    [http],
  );

  useEffect(() => {
    if (!providerId) {
      setModels([]);
      return;
    }
    load(providerId);
  }, [providerId, load]);

  return { models, loading };
}
