export function taskStatusBadgeVariant(status: string) {
  switch (status) {
    case "pending": return "outline" as const;
    case "in_progress": return "info" as const;
    case "completed": return "success" as const;
    case "blocked": return "warning" as const;
    default: return "outline" as const;
  }
}
