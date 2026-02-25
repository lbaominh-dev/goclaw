import { FileText } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import type { BootstrapFile } from "@/types/agent";

interface FileSidebarProps {
  files: BootstrapFile[];
  selectedFile: string | null;
  onSelect: (name: string) => void;
  isUserScoped: (name: string) => boolean;
}

export function FileSidebar({
  files,
  selectedFile,
  onSelect,
  isUserScoped,
}: FileSidebarProps) {
  return (
    <div className="w-52 space-y-1 overflow-y-auto border-r pr-4">
      {files.map((file) => {
        const userScoped = isUserScoped(file.name);
        return (
          <button
            key={file.name}
            type="button"
            onClick={() => !userScoped && onSelect(file.name)}
            disabled={userScoped}
            className={`flex w-full items-center gap-2 rounded-md px-2 py-2 text-sm transition-colors ${
              userScoped
                ? "cursor-not-allowed opacity-60"
                : selectedFile === file.name
                  ? "bg-accent text-accent-foreground"
                  : "hover:bg-muted"
            }`}
          >
            <FileText className="h-3.5 w-3.5 shrink-0" />
            <span className="min-w-0 flex-1 truncate text-left">
              {file.name}
            </span>
            {userScoped ? (
              <Badge variant="outline" className="shrink-0 text-[10px]">
                per-user
              </Badge>
            ) : file.missing ? (
              <Badge variant="outline" className="shrink-0 text-[10px]">
                empty
              </Badge>
            ) : (
              <span className="shrink-0 text-[10px] text-muted-foreground">
                {file.size > 1024
                  ? `${(file.size / 1024).toFixed(1)}K`
                  : `${file.size}B`}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}
