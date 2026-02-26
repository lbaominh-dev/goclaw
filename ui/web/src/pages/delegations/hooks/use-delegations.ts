import { useState, useCallback } from "react";
import { useHttp } from "@/hooks/use-ws";
import type { DelegationHistoryRecord, DelegationListFilters } from "@/types/delegation";

export function useDelegations() {
  const http = useHttp();
  const [delegations, setDelegations] = useState<DelegationHistoryRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);

  const load = useCallback(
    async (filters: DelegationListFilters = {}) => {
      setLoading(true);
      try {
        const params: Record<string, string> = {};
        if (filters.source_agent_id) params.source_agent_id = filters.source_agent_id;
        if (filters.target_agent_id) params.target_agent_id = filters.target_agent_id;
        if (filters.team_id) params.team_id = filters.team_id;
        if (filters.user_id) params.user_id = filters.user_id;
        if (filters.status) params.status = filters.status;
        if (filters.limit) params.limit = String(filters.limit);
        if (filters.offset !== undefined) params.offset = String(filters.offset);

        const res = await http.get<{ records: DelegationHistoryRecord[]; total?: number }>("/v1/delegations", params);
        setDelegations(res.records ?? []);
        setTotal(res.total ?? 0);
      } catch {
        // ignore
      } finally {
        setLoading(false);
      }
    },
    [http],
  );

  const getDelegation = useCallback(
    async (id: string): Promise<DelegationHistoryRecord | null> => {
      try {
        return await http.get<DelegationHistoryRecord>(`/v1/delegations/${id}`);
      } catch {
        return null;
      }
    },
    [http],
  );

  return { delegations, total, loading, load, getDelegation };
}
