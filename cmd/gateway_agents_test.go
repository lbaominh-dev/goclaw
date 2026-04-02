package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

func captureEmbeddingRequest(t *testing.T, es *store.EmbeddingSettings) map[string]any {
	t.Helper()

	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer server.Close()

	provider := &store.LLMProviderData{
		Name:         "embedding-provider",
		ProviderType: store.ProviderOpenAICompat,
		APIKey:       "test-key",
		APIBase:      server.URL,
		Enabled:      true,
	}

	ep := buildEmbeddingProvider(provider, es, nil, nil)
	if ep == nil {
		t.Fatal("buildEmbeddingProvider() = nil, want provider")
	}
	if _, err := ep.Embed(context.Background(), []string{"hello"}); err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	return requestBody
}

func TestBuildEmbeddingProviderDefaultsTo1536Dimensions(t *testing.T) {
	requestBody := captureEmbeddingRequest(t, nil)
	if got := requestBody["dimensions"]; got != float64(1536) {
		t.Fatalf("dimensions = %v, want 1536", got)
	}
}

func TestBuildEmbeddingProviderIgnoresIncompatibleStoredDimensions(t *testing.T) {
	requestBody := captureEmbeddingRequest(t, &store.EmbeddingSettings{
		Enabled:    true,
		Model:      "voyage-4-nano",
		Dimensions: 2048,
	})
	if got := requestBody["dimensions"]; got != float64(1536) {
		t.Fatalf("dimensions = %v, want fallback 1536", got)
	}
}

func TestSetupSubagentsPreservesReadFileSkillAllowPaths(t *testing.T) {
	workspace := t.TempDir()
	skillsDir := t.TempDir()
	skillDir := filepath.Join(skillsDir, "attached-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("# attached skill"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	providerReg := providers.NewRegistry(store.TenantIDFromContext)
	providerReg.Register(&stubProvider{name: "test-provider"})

	cfg := &config.Config{}
	cfg.Agents.Defaults = config.AgentDefaults{Provider: "test-provider", RestrictToWorkspace: true}

	toolsReg := tools.NewRegistry()
	toolsReg.Register(tools.NewReadFileTool(workspace, true))
	allowReadFileSkillPaths(toolsReg, readFilePathConfig{globalSkillsDir: skillsDir})

	if mgr := setupSubagents(providerReg, cfg, nil, toolsReg, workspace, nil, readFilePathConfig{globalSkillsDir: skillsDir}); mgr == nil {
		t.Fatal("setupSubagents() = nil, want manager")
	}

	reg := buildSubagentTools(toolsReg, workspace, true, nil, readFilePathConfig{globalSkillsDir: skillsDir})

	res := reg.Execute(context.Background(), "read_file", map[string]any{"path": skillFile})
	if res.IsError {
		t.Fatalf("read_file() error = %q, want success", res.ForLLM)
	}
	if res.ForLLM != "# attached skill" {
		t.Fatalf("read_file() = %q, want skill content", res.ForLLM)
	}
}

type stubProvider struct{ name string }

func (p *stubProvider) Chat(context.Context, providers.ChatRequest) (*providers.ChatResponse, error) {
	panic("not used in test")
}

func (p *stubProvider) ChatStream(context.Context, providers.ChatRequest, func(providers.StreamChunk)) (*providers.ChatResponse, error) {
	panic("not used in test")
}

func (p *stubProvider) DefaultModel() string { return "test-model" }

func (p *stubProvider) Name() string { return p.name }
