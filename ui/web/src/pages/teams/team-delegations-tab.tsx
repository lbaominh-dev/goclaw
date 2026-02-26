import { useState, useEffect } from "react";
import { ArrowRightLeft } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/shared/empty-state";
import { DeferredSpinner } from "@/components/shared/loading-skeleton";
import { useHttp } from "@/hooks/use-ws";
import { formatDate, formatDuration } from "@/lib/format";
import type { DelegationHistoryRecord } from "@/types/delegation";

interface TeamDelegationsTabProps {
  teamId: string;
}

export function TeamDelegationsTab({ teamId }: TeamDelegationsTabProps) {
  const http = useHttp();
  const [records, setRecords] = useState<DelegationHistoryRecord[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const res = await http.get<{ records: DelegationHistoryRecord[] }>("/v1/delegations", {
          team_id: teamId,
          limit: "50",
        });
        if (!cancelled) setRecords(res.records ?? []);
      } catch {
        // ignore
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [teamId, http]);

  if (loading) return <DeferredSpinner />;

  if (records.length === 0) {
    return (
      <EmptyState
        icon={ArrowRightLeft}
        title="No delegations"
        description="No delegation records found for this team."
      />
    );
  }

  return (
    <div className="rounded-md border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b bg-muted/50">
            <th className="px-4 py-3 text-left font-medium">Source / Target</th>
            <th className="px-4 py-3 text-left font-medium">Task</th>
            <th className="px-4 py-3 text-left font-medium">Status</th>
            <th className="px-4 py-3 text-left font-medium">Duration</th>
            <th className="px-4 py-3 text-left font-medium">Time</th>
          </tr>
        </thead>
        <tbody>
          {records.map((d) => (
            <tr key={d.id} className="border-b last:border-0">
              <td className="px-4 py-3">
                <span className="font-medium">{d.source_agent_key || d.source_agent_id.slice(0, 8)}</span>
                <span className="mx-1 text-muted-foreground">&rarr;</span>
                <span className="font-medium">{d.target_agent_key || d.target_agent_id.slice(0, 8)}</span>
              </td>
              <td className="max-w-[250px] truncate px-4 py-3 text-muted-foreground">
                {d.task}
              </td>
              <td className="px-4 py-3">
                <StatusBadge status={d.status} />
              </td>
              <td className="px-4 py-3 text-muted-foreground">
                {formatDuration(d.duration_ms)}
              </td>
              <td className="px-4 py-3 text-muted-foreground">
                {formatDate(d.created_at)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const variant =
    status === "completed"
      ? "success"
      : status === "failed"
        ? "destructive"
        : status === "running" || status === "pending"
          ? "info"
          : "secondary";

  return <Badge variant={variant} className="text-xs">{status || "unknown"}</Badge>;
}
