export function roleBadgeVariant(role: string) {
  switch (role) {
    case "lead": return "info" as const;
    case "member": return "outline" as const;
    default: return "outline" as const;
  }
}
