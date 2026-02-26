import { useState, useCallback } from "react";
import { useHttp } from "@/hooks/use-ws";

export interface TraceData {
  id: string;
  parent_trace_id?: string;
  agent_id?: string;
  user_id: string;
  session_key: string;
  run_id: string;
  start_time: string;
  end_time?: string;
  duration_ms: number;
  name: string;
  channel: string;
  input_preview: string;
  output_preview: string;
  total_input_tokens: number;
  total_output_tokens: number;
  total_cost: number;
  span_count: number;
  llm_call_count: number;
  tool_call_count: number;
  status: string;
  error?: string;
  tags?: string[];
  metadata?: { total_cache_read_tokens?: number; total_cache_creation_tokens?: number };
  created_at: string;
}

export interface SpanData {
  id: string;
  trace_id: string;
  parent_span_id?: string;
  agent_id?: string;
  span_type: string;
  name: string;
  start_time: string;
  end_time?: string;
  duration_ms: number;
  status: string;
  error?: string;
  model: string;
  provider: string;
  input_tokens: number;
  output_tokens: number;
  total_cost: number;
  finish_reason: string;
  tool_name: string;
  tool_call_id: string;
  input_preview: string;
  output_preview: string;
  metadata?: { cache_creation_tokens?: number; cache_read_tokens?: number };
}

interface TraceFilters {
  agentId?: string;
  userId?: string;
  status?: string;
  limit?: number;
  offset?: number;
}

export function useTraces() {
  const http = useHttp();
  const [traces, setTraces] = useState<TraceData[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);

  const load = useCallback(
    async (filters: TraceFilters = {}) => {
      setLoading(true);
      try {
        const params: Record<string, string> = {};
        if (filters.agentId) params.agent_id = filters.agentId;
        if (filters.userId) params.user_id = filters.userId;
        if (filters.status) params.status = filters.status;
        if (filters.limit) params.limit = String(filters.limit);
        if (filters.offset !== undefined) params.offset = String(filters.offset);

        const res = await http.get<{ traces: TraceData[]; total?: number }>("/v1/traces", params);
        setTraces(res.traces ?? []);
        setTotal(res.total ?? 0);
      } catch {
        // ignore
      } finally {
        setLoading(false);
      }
    },
    [http],
  );

  const getTrace = useCallback(
    async (traceId: string): Promise<{ trace: TraceData; spans: SpanData[] } | null> => {
      try {
        return await http.get<{ trace: TraceData; spans: SpanData[] }>(`/v1/traces/${traceId}`);
      } catch {
        return null;
      }
    },
    [http],
  );

  return { traces, total, loading, load, getTrace };
}
