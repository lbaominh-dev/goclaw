/** Delegation history types matching Go internal/store/team_store.go */

export interface DelegationHistoryRecord {
  id: string;
  source_agent_id: string;
  target_agent_id: string;
  team_id?: string;
  team_task_id?: string;
  user_id?: string;
  task: string;
  mode: string;
  status: string;
  result?: string;
  error?: string;
  iterations: number;
  trace_id?: string;
  duration_ms: number;
  completed_at?: string;
  created_at: string;
  // Joined fields
  source_agent_key?: string;
  target_agent_key?: string;
}

export interface DelegationListFilters {
  source_agent_id?: string;
  target_agent_id?: string;
  team_id?: string;
  user_id?: string;
  status?: string;
  limit?: number;
  offset?: number;
}
