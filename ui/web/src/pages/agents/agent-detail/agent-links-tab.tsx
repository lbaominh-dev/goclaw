import { useState, useEffect, useMemo } from "react";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { useAgentLinks } from "../hooks/use-agent-links";
import { useAgents } from "../hooks/use-agents";
import type { AgentLinkData } from "@/types/agent";
import { LinkCreateForm, LinkList, LinkEditDialog, linkTargetName } from "./link-sections";

interface AgentLinksTabProps {
  agentId: string;
}

export function AgentLinksTab({ agentId }: AgentLinksTabProps) {
  const { links, loading, load, createLink, updateLink, deleteLink } =
    useAgentLinks(agentId);
  const { agents } = useAgents();

  const [editLink, setEditLink] = useState<AgentLinkData | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<{
    id: string;
    name: string;
  } | null>(null);

  useEffect(() => {
    load();
  }, [load]);

  // Only predefined agents can be delegation targets (open agents have no agent-level context)
  const agentOptions = useMemo(
    () =>
      agents
        .filter((a) => a.id !== agentId && a.agent_type === "predefined")
        .map((a) => ({
          value: a.id,
          label: a.display_name || a.agent_key,
        })),
    [agents, agentId],
  );

  const handleStatusToggle = async (link: AgentLinkData) => {
    const newStatus = link.status === "active" ? "disabled" : "active";
    await updateLink(link.id, { status: newStatus });
  };

  return (
    <div className="max-w-4xl space-y-6">
      <LinkCreateForm agentOptions={agentOptions} onSubmit={createLink} />

      <LinkList
        links={links}
        loading={loading}
        agentId={agentId}
        onStatusToggle={handleStatusToggle}
        onEdit={setEditLink}
        onDelete={(link) =>
          setDeleteTarget({ id: link.id, name: linkTargetName(link, agentId) })
        }
      />

      <LinkEditDialog
        link={editLink}
        onClose={() => setEditLink(null)}
        onSave={updateLink}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={() => setDeleteTarget(null)}
        title="Delete Link"
        description={`Remove the delegation link to "${deleteTarget?.name}"? Agents will no longer be able to delegate to each other through this link.`}
        confirmLabel="Delete"
        variant="destructive"
        onConfirm={async () => {
          if (deleteTarget) {
            try {
              await deleteLink(deleteTarget.id);
            } catch {
              // ignore
            }
            setDeleteTarget(null);
          }
        }}
      />
    </div>
  );
}
