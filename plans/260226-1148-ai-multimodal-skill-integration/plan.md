---
status: completed
branch: goon
created: 2026-02-26
---

# Plan: Integrate ai-multimodal Skill into GoClaw

## Overview
Copy existing `ai-multimodal` Claude Code skill to GoClaw global skills directory (`~/.goclaw/skills/ai-multimodal/`). Adapt SKILL.md to use `{baseDir}` placeholders. No Go code changes needed.

## Source
`/Users/duynguyen/www/claudekit/claudekit-engineer/.claude/skills/ai-multimodal`

## Brainstorm Report
`plans/reports/brainstorm-260226-1148-ai-multimodal-skill-integration.md`

## Phases

| # | Phase | Status | Effort |
|---|-------|--------|--------|
| 1 | Copy & adapt skill files | done | small |
| 2 | Environment setup | done | small |
| 3 | Verification & testing | done | small |

## Phase 1: Copy & Adapt Skill Files

### Steps
1. Create `~/.goclaw/skills/ai-multimodal/` directory
2. Copy entire skill directory (SKILL.md, scripts/, references/)
3. Edit SKILL.md: replace all `scripts/` with `{baseDir}/scripts/`
4. Edit SKILL.md: replace `references/` with `{baseDir}/references/`
5. Remove Claude Code-specific frontmatter keys (`allowed-tools`, `license`) — GoClaw only uses `name`, `description`, `slug`

### Files
- Source: `/Users/duynguyen/www/claudekit/claudekit-engineer/.claude/skills/ai-multimodal/`
- Target: `~/.goclaw/skills/ai-multimodal/`

### Todo
- [ ] Create target directory
- [ ] Copy files
- [ ] Adapt SKILL.md paths to `{baseDir}`
- [ ] Clean frontmatter

## Phase 2: Environment Setup

### Steps
1. Install Python deps: `pip install google-genai python-dotenv pillow`
2. Set `GEMINI_API_KEY` env var on server
3. Optional: set `GEMINI_API_KEY_2`, `GEMINI_API_KEY_3` for rotation

### Todo
- [ ] Install Python packages
- [ ] Set API key(s)

## Phase 3: Verification & Testing

### Steps
1. Start GoClaw — verify skill appears in `skills.list` RPC
2. Test `skill_search` finds "ai-multimodal"
3. Test agent can exec `python {baseDir}/scripts/check_setup.py`
4. Test image analysis: agent analyzes a sample image
5. Verify `{baseDir}` resolves correctly in skill content

### Success Criteria
- Skill discoverable via search
- Agent can run Python scripts via exec tool
- `{baseDir}` resolves to `~/.goclaw/skills/ai-multimodal`

### Todo
- [ ] Verify skill listed
- [ ] Test search discovery
- [ ] Test exec tool invocation
- [ ] Test image analysis end-to-end
