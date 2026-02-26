import type { TeamMemberData } from "@/types/team";
import { MemberList } from "./member-sections";

interface TeamMembersTabProps {
  members: TeamMemberData[];
}

export function TeamMembersTab({ members }: TeamMembersTabProps) {
  return (
    <div className="max-w-2xl space-y-6">
      <MemberList members={members} />
    </div>
  );
}
