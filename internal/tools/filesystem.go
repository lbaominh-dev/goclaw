package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/sandbox"
)

// ReadFileTool reads file contents, optionally through a sandbox container.
type ReadFileTool struct {
	workspace       string
	restrict        bool
	allowedPrefixes []string              // extra allowed path prefixes (e.g. skills dirs)
	deniedPrefixes  []string              // path prefixes to deny access to (e.g. .goclaw)
	sandboxMgr      sandbox.Manager       // nil = direct host access
	contextFileIntc *ContextFileInterceptor // nil = no virtual FS routing (standalone mode)
	memIntc         *MemoryInterceptor      // nil = no memory routing (standalone mode)
}

// SetContextFileInterceptor enables virtual FS routing for context files (managed mode).
func (t *ReadFileTool) SetContextFileInterceptor(intc *ContextFileInterceptor) {
	t.contextFileIntc = intc
}

// SetMemoryInterceptor enables virtual FS routing for memory files (managed mode).
func (t *ReadFileTool) SetMemoryInterceptor(intc *MemoryInterceptor) {
	t.memIntc = intc
}

func NewReadFileTool(workspace string, restrict bool) *ReadFileTool {
	return &ReadFileTool{workspace: workspace, restrict: restrict}
}

// AllowPaths adds extra path prefixes that read_file is allowed to access
// even when restrict_to_workspace is true (e.g. skills directories).
func (t *ReadFileTool) AllowPaths(prefixes ...string) {
	t.allowedPrefixes = append(t.allowedPrefixes, prefixes...)
}

// DenyPaths adds path prefixes that read_file must reject (e.g. hidden dirs).
func (t *ReadFileTool) DenyPaths(prefixes ...string) {
	t.deniedPrefixes = append(t.deniedPrefixes, prefixes...)
}

func NewSandboxedReadFileTool(workspace string, restrict bool, mgr sandbox.Manager) *ReadFileTool {
	return &ReadFileTool{workspace: workspace, restrict: restrict, sandboxMgr: mgr}
}

// SetSandboxKey is a no-op; sandbox key is now read from ctx (thread-safe).
func (t *ReadFileTool) SetSandboxKey(key string) {}

func (t *ReadFileTool) Name() string        { return "read_file" }
func (t *ReadFileTool) Description() string { return "Read the contents of a file" }
func (t *ReadFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to read",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, args map[string]interface{}) *Result {
	path, _ := args["path"].(string)
	if path == "" {
		return ErrorResult("path is required")
	}

	// Virtual FS: route context files to DB (managed mode)
	if t.contextFileIntc != nil {
		if content, handled, err := t.contextFileIntc.ReadFile(ctx, path); handled {
			if err != nil {
				return ErrorResult(fmt.Sprintf("failed to read context file: %v", err))
			}
			if content == "" {
				return ErrorResult(fmt.Sprintf("context file not found: %s", path))
			}
			return SilentResult(content)
		}
	}

	// Virtual FS: route memory files to DB (managed mode)
	if t.memIntc != nil {
		if content, handled, err := t.memIntc.ReadFile(ctx, path); handled {
			if err != nil {
				return ErrorResult(fmt.Sprintf("failed to read memory file: %v", err))
			}
			if content == "" {
				return SilentResult(fmt.Sprintf("(memory file %s does not exist yet — it will be created when memory is saved)", path))
			}
			return SilentResult(content)
		}
	}

	// Sandbox routing (sandboxKey from ctx — thread-safe)
	sandboxKey := ToolSandboxKeyFromCtx(ctx)
	if t.sandboxMgr != nil && sandboxKey != "" {
		return t.executeInSandbox(ctx, path, sandboxKey)
	}

	// Host execution — use per-user workspace from context if available (managed mode)
	workspace := ToolWorkspaceFromCtx(ctx)
	if workspace == "" {
		workspace = t.workspace
	}
	resolved, err := resolvePathWithAllowed(path, workspace, t.restrict, t.allowedPrefixes)
	if err != nil {
		return ErrorResult(err.Error())
	}
	if err := checkDeniedPath(resolved, t.workspace, t.deniedPrefixes); err != nil {
		return ErrorResult(err.Error())
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read file: %v", err))
	}

	return SilentResult(string(data))
}

func (t *ReadFileTool) executeInSandbox(ctx context.Context, path, sandboxKey string) *Result {
	bridge, err := t.getFsBridge(ctx, sandboxKey)
	if err != nil {
		return ErrorResult(fmt.Sprintf("sandbox error: %v", err))
	}

	data, err := bridge.ReadFile(ctx, path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read file: %v", err))
	}

	return SilentResult(data)
}

func (t *ReadFileTool) getFsBridge(ctx context.Context, sandboxKey string) (*sandbox.FsBridge, error) {
	sb, err := t.sandboxMgr.Get(ctx, sandboxKey, t.workspace)
	if err != nil {
		return nil, err
	}
	return sandbox.NewFsBridge(sb.ID(), "/workspace"), nil
}

// resolvePathWithAllowed is like resolvePath but also allows paths under extra prefixes.
func resolvePathWithAllowed(path, workspace string, restrict bool, allowedPrefixes []string) (string, error) {
	resolved, err := resolvePath(path, workspace, restrict)
	if err == nil {
		return resolved, nil
	}
	// If restricted and denied, check if path falls under an allowed prefix.
	cleaned := filepath.Clean(path)
	for _, prefix := range allowedPrefixes {
		absPrefix, _ := filepath.Abs(prefix)
		if strings.HasPrefix(cleaned, absPrefix) {
			slog.Debug("read_file: allowed by prefix", "path", cleaned, "prefix", absPrefix)
			return cleaned, nil
		}
	}
	slog.Warn("read_file: access denied", "path", cleaned, "workspace", workspace, "allowedPrefixes", allowedPrefixes)
	return "", err
}

// checkDeniedPath returns an error if the resolved path falls under any denied prefix.
// Denied prefixes are relative to the workspace (e.g. ".goclaw" denies workspace/.goclaw/).
func checkDeniedPath(resolved, workspace string, deniedPrefixes []string) error {
	if len(deniedPrefixes) == 0 {
		return nil
	}
	absResolved, _ := filepath.Abs(resolved)
	absWorkspace, _ := filepath.Abs(workspace)
	for _, prefix := range deniedPrefixes {
		denied := filepath.Join(absWorkspace, prefix)
		if strings.HasPrefix(absResolved, denied) {
			return fmt.Errorf("access denied: path %s is restricted", prefix)
		}
	}
	return nil
}

// resolvePath resolves a path relative to the workspace and validates it.
func resolvePath(path, workspace string, restrict bool) (string, error) {
	if filepath.IsAbs(path) {
		if restrict {
			absWorkspace, _ := filepath.Abs(workspace)
			if !strings.HasPrefix(path, absWorkspace) {
				return "", fmt.Errorf("access denied: path outside workspace")
			}
		}
		return filepath.Clean(path), nil
	}

	resolved := filepath.Join(workspace, path)
	resolved = filepath.Clean(resolved)

	if restrict {
		absWorkspace, _ := filepath.Abs(workspace)
		absResolved, _ := filepath.Abs(resolved)
		if !strings.HasPrefix(absResolved, absWorkspace) {
			return "", fmt.Errorf("access denied: path outside workspace")
		}
	}

	return resolved, nil
}
