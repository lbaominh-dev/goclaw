import { Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { FILE_DESCRIPTIONS } from "./file-utils";

interface FileEditorProps {
  fileName: string | null;
  content: string;
  onChange: (content: string) => void;
  loading: boolean;
  dirty: boolean;
  saving: boolean;
  canEdit: boolean;
  onSave: () => void;
}

export function FileEditor({
  fileName,
  content,
  onChange,
  loading,
  dirty,
  saving,
  canEdit,
  onSave,
}: FileEditorProps) {
  if (!fileName) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        Select a file to {canEdit ? "edit" : "view"}
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col">
      <div className="mb-2 flex items-center justify-between">
        <div>
          <span className="text-sm font-medium">{fileName}</span>
          {FILE_DESCRIPTIONS[fileName] && (
            <span className="ml-2 text-xs text-muted-foreground">
              - {FILE_DESCRIPTIONS[fileName]}
            </span>
          )}
        </div>
        {canEdit && (
          <Button size="sm" onClick={onSave} disabled={!dirty || saving}>
            {!saving && <Save className="h-3.5 w-3.5" />}
            {saving ? "Saving..." : "Save"}
          </Button>
        )}
      </div>
      {loading && !content ? (
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          Loading...
        </div>
      ) : (
        <Textarea
          value={content}
          onChange={(e) => {
            if (!canEdit) return;
            onChange(e.target.value);
          }}
          readOnly={!canEdit}
          className={`flex-1 resize-none font-mono text-sm ${!canEdit ? "opacity-70" : ""}`}
          placeholder="File content..."
        />
      )}
    </div>
  );
}
