package http

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bootstrap"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// Summoning event type constants.
const (
	SummonEventStarted       = "started"
	SummonEventFailed        = "failed"
	SummonEventCompleted     = "completed"
	SummonEventFileGenerated = "file_generated"
)

// frontmatterKey is the special key used to store frontmatter in the parsed file map.
const frontmatterKey = "__frontmatter__"

// summoningFiles is the ordered list of context files the LLM should generate.
var summoningFiles = []string{
	bootstrap.SoulFile,
	bootstrap.IdentityFile,
	bootstrap.AgentsFile,
	bootstrap.ToolsFile,
	bootstrap.HeartbeatFile,
}

// fileTagRe parses <file name="SOUL.md">content</file> from LLM output.
var fileTagRe = regexp.MustCompile(`(?s)<file\s+name="([^"]+)">\s*(.*?)\s*</file>`)

// frontmatterTagRe parses <frontmatter>short expertise summary</frontmatter> from LLM output.
var frontmatterTagRe = regexp.MustCompile(`(?s)<frontmatter>\s*(.*?)\s*</frontmatter>`)

// AgentSummoner generates context files for predefined agents using an LLM.
// Runs one-shot background calls — no session data, no agent loop.
type AgentSummoner struct {
	agents      store.AgentStore
	providerReg *providers.Registry
	msgBus      *bus.MessageBus
}

// NewAgentSummoner creates a summoner backed by the given stores and provider registry.
func NewAgentSummoner(agents store.AgentStore, providerReg *providers.Registry, msgBus *bus.MessageBus) *AgentSummoner {
	return &AgentSummoner{
		agents:      agents,
		providerReg: providerReg,
		msgBus:      msgBus,
	}
}

// SummonAgent generates context files from a natural language description.
// Meant to be called as a goroutine: go summoner.SummonAgent(...)
// On success: stores generated files and sets agent status to "active".
// On failure: keeps template files (already seeded) and sets status to store.AgentStatusSummonFailed.
func (s *AgentSummoner) SummonAgent(agentID uuid.UUID, providerName, model, description string) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	s.emitEvent(agentID, SummonEventStarted, "", "")

	files, err := s.generateFiles(ctx, providerName, model, s.buildCreatePrompt(description))
	if err != nil {
		slog.Warn("summoning: LLM generation failed, falling back to templates",
			"agent", agentID, "error", err)
		s.emitEvent(agentID, SummonEventFailed, "", err.Error())
		// Use fresh context — the original may have timed out, but we still need to update status.
		s.setAgentStatus(context.Background(), agentID, store.AgentStatusSummonFailed)
		return
	}

	s.storeFiles(ctx, agentID, files)

	// Auto-generate frontmatter from description if LLM included it
	if fm, ok := files[frontmatterKey]; ok && fm != "" {
		if err := s.agents.Update(ctx, agentID, map[string]any{"frontmatter": fm}); err != nil {
			slog.Warn("summoning: failed to save frontmatter", "agent", agentID, "error", err)
		}
	}

	s.setAgentStatus(ctx, agentID, store.AgentStatusActive)
	s.emitEvent(agentID, SummonEventCompleted, "", "")

	slog.Info("summoning: completed", "agent", agentID, "files", len(files))
}

// RegenerateAgent updates context files based on an edit prompt.
// Reads existing files, sends them + edit instructions to LLM, stores results.
// Synchronous — caller should run in goroutine if needed.
func (s *AgentSummoner) RegenerateAgent(agentID uuid.UUID, providerName, model, editPrompt string) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	s.emitEvent(agentID, SummonEventStarted, "", "")

	// Read existing files for context
	existing, err := s.agents.GetAgentContextFiles(ctx, agentID)
	if err != nil {
		slog.Warn("summoning: failed to read existing files", "agent", agentID, "error", err)
		s.emitEvent(agentID, SummonEventFailed, "", err.Error())
		s.setAgentStatus(context.Background(), agentID, store.AgentStatusSummonFailed)
		return
	}

	prompt := s.buildEditPrompt(existing, editPrompt)

	files, err := s.generateFiles(ctx, providerName, model, prompt)
	if err != nil {
		slog.Warn("summoning: regeneration failed", "agent", agentID, "error", err)
		s.emitEvent(agentID, SummonEventFailed, "", err.Error())
		// Use fresh context — the original may have timed out, but we still need to update status.
		s.setAgentStatus(context.Background(), agentID, store.AgentStatusSummonFailed)
		return
	}

	s.storeFiles(ctx, agentID, files)
	s.setAgentStatus(ctx, agentID, store.AgentStatusActive)
	s.emitEvent(agentID, SummonEventCompleted, "", "")

	slog.Info("summoning: regeneration completed", "agent", agentID, "files", len(files))
}

// generateFiles calls the LLM and parses the XML-tagged response into file map.
func (s *AgentSummoner) generateFiles(ctx context.Context, providerName, model, prompt string) (map[string]string, error) {
	provider, err := s.resolveProvider(providerName)
	if err != nil {
		return nil, fmt.Errorf("resolve provider: %w", err)
	}

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{
			{Role: "user", Content: prompt},
		},
		Model: model,
		Options: map[string]interface{}{
			"max_tokens":  8192,
			"temperature": 0.7,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}

	files := parseFileResponse(resp.Content)
	if len(files) == 0 {
		return nil, fmt.Errorf("LLM returned no parseable files (response length: %d)", len(resp.Content))
	}

	return files, nil
}

// storeFiles saves generated files to agent_context_files and emits progress events.
func (s *AgentSummoner) storeFiles(ctx context.Context, agentID uuid.UUID, files map[string]string) {
	for _, name := range summoningFiles {
		content, ok := files[name]
		if !ok || content == "" {
			continue
		}
		if err := s.agents.SetAgentContextFile(ctx, agentID, name, content); err != nil {
			slog.Warn("summoning: failed to store file", "agent", agentID, "file", name, "error", err)
			continue
		}
		s.emitEvent(agentID, SummonEventFileGenerated, name, "")
	}
}

func (s *AgentSummoner) resolveProvider(name string) (providers.Provider, error) {
	if s.providerReg == nil {
		return nil, fmt.Errorf("no provider registry")
	}

	provider, err := s.providerReg.Get(name)
	if err != nil {
		// Fallback to first available provider
		names := s.providerReg.List()
		if len(names) == 0 {
			return nil, fmt.Errorf("no providers configured")
		}
		provider, err = s.providerReg.Get(names[0])
		if err != nil {
			return nil, err
		}
		slog.Warn("summoning: provider not found, using fallback", "wanted", name, "using", names[0])
	}
	return provider, nil
}

func (s *AgentSummoner) setAgentStatus(ctx context.Context, agentID uuid.UUID, status string) {
	if err := s.agents.Update(ctx, agentID, map[string]any{"status": status}); err != nil {
		slog.Warn("summoning: failed to update agent status", "agent", agentID, "status", status, "error", err)
	}
}

func (s *AgentSummoner) emitEvent(agentID uuid.UUID, eventType, fileName, errMsg string) {
	if s.msgBus == nil {
		return
	}
	payload := map[string]interface{}{
		"type":     eventType,
		"agent_id": agentID.String(),
	}
	if fileName != "" {
		payload["file"] = fileName
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	s.msgBus.Broadcast(bus.Event{
		Name:    protocol.EventAgentSummoning,
		Payload: payload,
	})
}

// buildCreatePrompt constructs the system + user prompt for initial file generation.
// Includes the full template files as reference so the LLM preserves core operational structure.
func (s *AgentSummoner) buildCreatePrompt(description string) string {
	// Load templates as reference material
	templates := make(map[string]string)
	for _, name := range summoningFiles {
		content, err := bootstrap.ReadTemplate(name)
		if err != nil {
			slog.Warn("summoning: failed to read template for prompt", "file", name, "error", err)
			continue
		}
		templates[name] = content
	}

	var sb strings.Builder
	sb.WriteString("You are setting up a new AI assistant. Based on the description below, generate customized content for each context file.\n\n")

	fmt.Fprintf(&sb, "<description>\n%s\n</description>\n\n", description)

	sb.WriteString("Below are the DEFAULT TEMPLATES for each file. Use them as the foundation — preserve the core structure and operational rules, but customize the content to match the agent's purpose and personality.\n\n")

	sb.WriteString("<templates>\n")
	for _, name := range summoningFiles {
		if content, ok := templates[name]; ok {
			fmt.Fprintf(&sb, "<file name=%q>\n%s\n</file>\n", name, content)
		}
	}
	sb.WriteString("</templates>\n\n")

	sb.WriteString(`IMPORTANT — Language rule: You MUST write ALL file content in the SAME LANGUAGE as the <description> above. If the description is in Vietnamese, write in Vietnamese. If in English, write in English. The templates below are in English — translate and adapt them to match the description's language. Only keep technical terms (file names, code, commands) in English.

Instructions for each file:

- **SOUL.md**: Rewrite to reflect this agent's unique personality, values, communication style, and boundaries. Keep the spirit of the template (genuine helpfulness, opinions, resourcefulness) but make it specific to this agent's role.
- **IDENTITY.md**: Fill in the identity card fields (Name, Creature, Vibe, Emoji) based on the description. Leave Avatar blank.
- **AGENTS.md**: This is CRITICAL — you MUST preserve the core operational sections (First Run, Every Session, Memory, Safety, External vs Internal, Group Chats, Heartbeats). Customize the content within each section to fit the agent's purpose, but do NOT remove any section. The operational structure is required for the system to function.
- **TOOLS.md**: Customize with tool notes relevant to this agent's role. Keep the structure.
- **HEARTBEAT.md**: Add periodic tasks if relevant to the agent's role. Leave minimal if not applicable.

First, generate a short expertise summary (1-2 sentences, under 200 characters) describing what this agent specializes in. This is used for delegation discovery — other agents use it to decide whether to delegate tasks here.

<frontmatter>
(short expertise summary here)
</frontmatter>

Then generate each file inside XML tags:

<file name="SOUL.md">
(generate here)
</file>
<file name="IDENTITY.md">
(generate here)
</file>
<file name="AGENTS.md">
(generate here)
</file>
<file name="TOOLS.md">
(generate here)
</file>
<file name="HEARTBEAT.md">
(generate here)
</file>`)

	return sb.String()
}

// buildEditPrompt constructs the prompt for editing existing files.
func (s *AgentSummoner) buildEditPrompt(existing []store.AgentContextFileData, editPrompt string) string {
	var sb strings.Builder
	sb.WriteString("You are updating an existing AI assistant's configuration files.\n\nHere are the current files:\n\n<current_files>\n")
	for _, f := range existing {
		if f.Content == "" {
			continue
		}
		fmt.Fprintf(&sb, "<file name=%q>\n%s\n</file>\n", f.FileName, f.Content)
	}
	sb.WriteString("</current_files>\n\n")
	fmt.Fprintf(&sb, "<edit_instructions>\n%s\n</edit_instructions>\n\n", editPrompt)
	sb.WriteString("IMPORTANT — Language rule: Write ALL content in the SAME LANGUAGE as the existing files above. If the current files are in Vietnamese, write in Vietnamese. Only keep technical terms (file names, code, commands) in English.\n\n")
	sb.WriteString("Generate updated files. Only include files that need changes. Keep the same XML format:\n\n")
	sb.WriteString("<file name=\"SOUL.md\">\n(updated content, or omit if unchanged)\n</file>\n")
	sb.WriteString("...\n")
	return sb.String()
}

// parseFileResponse extracts file contents and frontmatter from XML-tagged LLM output.
// Frontmatter is stored under the special key "__frontmatter__".
func parseFileResponse(content string) map[string]string {
	files := make(map[string]string)
	matches := fileTagRe.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		name := strings.TrimSpace(m[1])
		body := strings.TrimSpace(m[2])
		if name != "" && body != "" {
			files[name] = body
		}
	}
	// Extract frontmatter tag if present
	if fm := frontmatterTagRe.FindStringSubmatch(content); len(fm) > 1 {
		if trimmed := strings.TrimSpace(fm[1]); trimmed != "" {
			files[frontmatterKey] = trimmed
		}
	}
	return files
}
