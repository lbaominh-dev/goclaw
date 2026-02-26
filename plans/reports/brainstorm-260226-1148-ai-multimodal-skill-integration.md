# Brainstorm: ai-multimodal Skill Integration for GoClaw

## Problem Statement
Integrate existing `ai-multimodal` Claude Code skill (Python + Gemini API) into GoClaw bot as a pure SKILL.md skill at global tier (`~/.goclaw/skills/`).

## Requirements
- All features: image analysis/OCR, image generation (Imagen 4), audio transcription, video analysis/generation (Veo)
- Agent has exec tool access — can run Python scripts
- Hot-reload via GoClaw skill watcher

## Evaluated Approaches

### 1. Skill thuần (SKILL.md + scripts) ✅ CHOSEN
- **Pros**: Zero Go changes, reuse existing Python scripts, hot-reload, fast to implement
- **Cons**: Python runtime dependency on server, env var setup needed

### 2. Native Go tool
- **Pros**: No Python dependency, deeper integration, better error handling
- **Cons**: Significant Go code to write, reimplementing existing Python logic

### 3. Hybrid (SKILL.md + Go wrapper)
- **Pros**: Flexible
- **Cons**: Over-engineered for current needs, YAGNI

## Final Solution

Copy `ai-multimodal/` skill to `~/.goclaw/skills/ai-multimodal/` with adapted SKILL.md using `{baseDir}` placeholders.

### Key Implementation Steps
1. Copy skill directory to `~/.goclaw/skills/ai-multimodal/`
2. Adapt SKILL.md: replace `scripts/` paths with `{baseDir}/scripts/`
3. Install Python deps: `pip install google-genai python-dotenv pillow`
4. Set `GEMINI_API_KEY` in server environment
5. Verify hot-reload picks up skill
6. Test via agent exec tool

### Risks
- **Python not installed on server**: Mitigate by documenting prereqs
- **API key rotation**: Skill supports multi-key rotation, need env vars set
- **Large media files**: Agent needs guidance on File API vs inline (20MB limit)
- **Exec tool security**: Ensure shell deny patterns don't block Python invocations

### Success Criteria
- Agent can discover skill via `skill_search`
- Agent can analyze images, generate images, transcribe audio, analyze/generate video
- `{baseDir}` correctly resolves to skill directory
- Hot-reload works on SKILL.md changes
