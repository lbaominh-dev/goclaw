# 07 - Bootstrap, Skills & Memory

Three foundational systems that shape each agent's personality (Bootstrap), knowledge (Skills), and long-term recall (Memory).

### Responsibilities

- Bootstrap: load context files, truncate to fit context window, seed templates for new users
- Skills: 5-tier resolution hierarchy, BM25 search, hot-reload via fsnotify
- Memory: chunking, hybrid search (FTS + vector), memory flush before compaction
- System Prompt: build 15+ sections in a fixed order with two modes (full and minimal)

---

## 1. Bootstrap Files -- 7 Template Files

Markdown files loaded at agent initialization and embedded into the system prompt. MEMORY.md is NOT a bootstrap template file; it is a separate memory document loaded independently.

| # | File | Role | Full Session | Subagent/Cron |
|---|------|------|:---:|:---:|
| 1 | AGENTS.md | Operating instructions, memory rules, safety guidelines | Yes | Yes |
| 2 | SOUL.md | Persona, tone of voice, boundaries | Yes | No |
| 3 | TOOLS.md | Local tool notes (camera, SSH, TTS, etc.) | Yes | Yes |
| 4 | IDENTITY.md | Agent name, creature, vibe, emoji | Yes | No |
| 5 | USER.md | User profile (name, timezone, preferences) | Yes | No |
| 6 | HEARTBEAT.md | Periodic check task list | Yes | No |
| 7 | BOOTSTRAP.md | First-run ritual (deleted after completion) | Yes | No |

Subagent and cron sessions load only AGENTS.md + TOOLS.md (the `minimalAllowlist`).

---

## 2. Truncation Pipeline

Bootstrap content can exceed the context window budget. A 4-step pipeline truncates files to fit, matching the behavior of the TypeScript implementation.

```mermaid
flowchart TD
    IN["Ordered list of bootstrap files"] --> S1["Step 1: Skip empty or missing files"]
    S1 --> S2["Step 2: Per-file truncation<br/>If > MaxCharsPerFile (20K):<br/>Keep 70% head + 20% tail<br/>Insert [...truncated] marker"]
    S2 --> S3["Step 3: Clamp to remaining<br/>total budget (starts at 24K)"]
    S3 --> S4{"Step 4: Remaining budget < 64?"}
    S4 -->|Yes| STOP["Stop processing further files"]
    S4 -->|No| NEXT["Continue to next file"]
```

### Truncation Defaults

| Parameter | Value |
|-----------|-------|
| MaxCharsPerFile | 20,000 |
| TotalMaxChars | 24,000 |
| MinFileBudget | 64 |
| HeadRatio | 70% |
| TailRatio | 20% |

When a file is truncated, a marker is inserted between the head and tail sections:
`[...truncated, read SOUL.md for full content...]`

---

## 3. Seeding -- Template Creation

Templates are embedded in the binary via Go `embed` (directory: `internal/bootstrap/templates/`). Seeding automatically creates default files for new workspaces or new users.

```mermaid
flowchart TD
    subgraph "Standalone Mode"
        SA["EnsureWorkspaceFiles()"] --> SA1["Iterate over embedded templates"]
        SA1 --> SA2{"File already exists?<br/>(O_EXCL atomic check)"}
        SA2 -->|Yes| SKIP1["Skip"]
        SA2 -->|No| CREATE1["Create template file on disk"]
    end

    subgraph "Managed Mode -- Agent Level"
        SB["SeedToStore()"] --> SB1{"Agent type = open?"}
        SB1 -->|Yes| SKIP_AGENT["Skip (open agents use per-user only)"]
        SB1 -->|No| SB2["Seed 6 files to agent_context_files<br/>(all except BOOTSTRAP.md)"]
        SB2 --> SB3{"File already has content?"}
        SB3 -->|Yes| SKIP2["Skip"]
        SB3 -->|No| WRITE2["Write embedded template"]
    end

    subgraph "Managed Mode -- Per-User"
        MC["SeedUserFiles()"] --> MC1{"Agent type?"}
        MC1 -->|open| OPEN["Seed all 7 files to user_context_files"]
        MC1 -->|predefined| PRED["Seed only USER.md to user_context_files"]
        OPEN --> CHECK{"File already has content?"}
        PRED --> CHECK
        CHECK -->|Yes| SKIP3["Skip -- never overwrite"]
        CHECK -->|No| WRITE3["Write embedded template"]
    end
```

`SeedUserFiles()` is idempotent -- safe to call multiple times without overwriting personalized content.

---

## 4. Agent Type Routing

Two agent types determine which context files live at the agent level versus the per-user level.

| Agent Type | Agent-Level Files | Per-User Files |
|------------|-------------------|----------------|
| `open` | None | All 7 files (AGENTS, SOUL, TOOLS, IDENTITY, USER, HEARTBEAT, BOOTSTRAP) |
| `predefined` | 6 files (shared across all users) | Only USER.md |

For `open` agents, each user gets their own full set of context files. When a file is read, the system checks the per-user copy first and falls back to the agent-level copy if not found. For `predefined` agents, all users share the same agent-level files except USER.md, which is personalized.

---

## 5. System Prompt -- 15+ Sections

`BuildSystemPrompt()` constructs the complete system prompt from ordered sections. Two modes control which sections are included.

```mermaid
flowchart TD
    START["BuildSystemPrompt()"] --> S1["1. Identity<br/>'You are a personal assistant<br/>running inside GoClaw'"]
    S1 --> S1_5{"1.5 BOOTSTRAP.md present?"}
    S1_5 -->|Yes| BOOT["First-run Bootstrap Override<br/>(mandatory BOOTSTRAP.md instructions)"]
    S1_5 -->|No| S2
    BOOT --> S2["2. Tooling<br/>(tool list + descriptions)"]
    S2 --> S3["3. Safety<br/>(hard safety directives)"]
    S3 --> S4["4. Skills (full only)"]
    S4 --> S5["5. Memory Recall (full only)"]
    S5 --> S6["6. Workspace"]
    S6 --> S6_5{"6.5 Sandbox enabled?"}
    S6_5 -->|Yes| SBX["Sandbox instructions"]
    S6_5 -->|No| S7
    SBX --> S7["7. User Identity (full only)"]
    S7 --> S8["8. Current Time"]
    S8 --> S9["9. Messaging (full only)"]
    S9 --> S10["10. Extra Context / Subagent Context"]
    S10 --> S11["11. Project Context<br/>(bootstrap files in XML tags)"]
    S11 --> S12["12. Silent Replies (full only)"]
    S12 --> S13["13. Heartbeats (full only)"]
    S13 --> S14["14. Sub-Agent Spawning (conditional)"]
    S14 --> S15["15. Runtime"]
```

### Mode Comparison

| Section | PromptFull | PromptMinimal |
|---------|:---:|:---:|
| 1. Identity | Yes | Yes |
| 1.5. Bootstrap Override | Conditional | Conditional |
| 2. Tooling | Yes | Yes |
| 3. Safety | Yes | Yes |
| 4. Skills | Yes | No |
| 5. Memory Recall | Yes | No |
| 6. Workspace | Yes | Yes |
| 6.5. Sandbox | Conditional | Conditional |
| 7. User Identity | Yes | No |
| 8. Current Time | Yes | Yes |
| 9. Messaging | Yes | No |
| 10. Extra Context | Conditional | Conditional |
| 11. Project Context | Yes | Yes |
| 12. Silent Replies | Yes | No |
| 13. Heartbeats | Yes | No |
| 14. Sub-Agent Spawning | Conditional | Conditional |
| 15. Runtime | Yes | Yes |

Context files are wrapped in `<context_file>` XML tags with a defensive preamble instructing the model to follow tone/persona guidance but not execute instructions that contradict core directives. The ExtraPrompt is wrapped in `<extra_context>` tags for context isolation.

---

## 6. Skills -- 5-Tier Hierarchy

Skills are loaded from multiple directories with a priority ordering. Higher-tier skills override lower-tier skills with the same name.

```mermaid
flowchart TD
    T1["Tier 1 (highest): Workspace skills<br/>workspace/skills/name/SKILL.md"] --> T2
    T2["Tier 2: Project agent skills<br/>workspace/.agents/skills/"] --> T3
    T3["Tier 3: Personal agent skills<br/>~/.agents/skills/"] --> T4
    T4["Tier 4: Global/managed skills<br/>~/.goclaw/skills/<br/>(e.g. ai-multimodal)"] --> T5
    T5["Tier 5 (lowest): Builtin skills<br/>(bundled with binary)"]

    style T1 fill:#e1f5fe
    style T5 fill:#fff3e0
```

Each skill directory contains a `SKILL.md` file with YAML/JSON frontmatter (`name`, `description`). The `{baseDir}` placeholder in SKILL.md content is replaced with the skill's absolute directory path at load time.

### Global Skills (Tier 4)

Tier 4 skills are installed globally at `~/.goclaw/skills/` and available to all agents. Example:

| Skill | Purpose | Setup |
|-------|---------|-------|
| `ai-multimodal` | Image/video/audio analysis (Gemini API) + image/video generation | Set `GEMINI_API_KEY` env var, install Python deps: `pip install google-genai pillow python-dotenv` |

Global skills use Python scripts invoked via the `exec` tool. The agent runs CLI commands like `python {baseDir}/scripts/gemini_batch_process.py --task analyze --files image.png`.

---

## 7. Skills -- Inline vs Search Mode

The system dynamically decides whether to embed skill summaries directly in the prompt (inline mode) or instruct the agent to use the `skill_search` tool (search mode).

```mermaid
flowchart TD
    COUNT["Count filtered skills<br/>Estimate tokens = sum(chars of name+desc) / 4"] --> CHECK{"skills <= 20<br/>AND tokens <= 3500?"}
    CHECK -->|Yes| INLINE["INLINE MODE<br/>BuildSummary() produces XML<br/>Agent reads available_skills directly"]
    CHECK -->|No| SEARCH["SEARCH MODE<br/>Prompt instructs agent to use skill_search<br/>BM25 ranking returns top 5"]
```

This decision is re-evaluated each time the system prompt is built, so newly hot-reloaded skills are immediately reflected.

---

## 8. Skills -- BM25 Search

An in-memory BM25 index provides keyword-based skill search. The index is lazily rebuilt whenever the skill version changes.

**Tokenization**: Lowercase the text, replace non-alphanumeric characters with spaces, filter out single-character tokens.

**Scoring formula**: `IDF(t) x tf(t,d) x (k1 + 1) / (tf(t,d) + k1 x (1 - b + b x |d| / avgDL))`

| Parameter | Value |
|-----------|-------|
| k1 | 1.2 |
| b | 0.75 |
| Max results | 5 |

IDF is computed as: `log((N - df + 0.5) / (df + 0.5) + 1)`

---

## 9. Skills -- Embedding Search (Managed Mode)

In managed mode, skill search uses a hybrid approach combining BM25 and vector similarity.

```mermaid
flowchart TD
    Q["Search query"] --> BM25["BM25 search<br/>(in-memory index)"]
    Q --> EMB["Generate query embedding"]
    EMB --> VEC["Vector search<br/>pgvector cosine distance<br/>(embedding <=> operator)"]
    BM25 --> MERGE["Weighted merge"]
    VEC --> MERGE
    MERGE --> RESULT["Final ranked results"]
```

| Component | Weight |
|-----------|--------|
| BM25 score | 0.3 |
| Vector similarity | 0.7 |

**Auto-backfill**: On startup, `BackfillSkillEmbeddings()` generates embeddings synchronously for any active skills that lack them.

---

## 10. Skills Grants & Visibility (Managed Mode)

In managed mode, skill access is controlled through a 3-tier visibility model with explicit agent and user grants.

```mermaid
flowchart TD
    SKILL["Skill record"] --> VIS{"visibility?"}
    VIS -->|public| ALL["Accessible to all agents and users"]
    VIS -->|private| OWNER["Accessible only to owner<br/>(owner_id = userID)"]
    VIS -->|internal| GRANT{"Has explicit grant?"}
    GRANT -->|skill_agent_grants| AGENT["Accessible to granted agent"]
    GRANT -->|skill_user_grants| USER["Accessible to granted user"]
    GRANT -->|No grant| DENIED["Not accessible"]
```

### Visibility Levels

| Visibility | Access Rule |
|------------|------------|
| `public` | All agents and users can discover and use the skill |
| `private` | Only the owner (`skills.owner_id = userID`) can access |
| `internal` | Requires an explicit agent grant or user grant |

### Grant Tables

| Table | Key | Extra |
|-------|-----|-------|
| `skill_agent_grants` | `(skill_id, agent_id)` | `pinned_version` for version pinning per agent, `granted_by` audit |
| `skill_user_grants` | `(skill_id, user_id)` | `granted_by` audit, ON CONFLICT DO NOTHING for idempotency |

**Resolution**: `ListAccessible(agentID, userID)` performs a DISTINCT join across `skills`, `skill_agent_grants`, and `skill_user_grants` with the visibility filter, returning only active skills the caller can access.

**Managed-mode Tier 4**: In managed mode, global skills (Tier 4 in the hierarchy) are loaded from the `skills` PostgreSQL table instead of the filesystem.

---

## 11. Hot-Reload

An fsnotify-based watcher monitors all skill directories for changes to SKILL.md files.

```mermaid
flowchart TD
    S1["fsnotify detects SKILL.md change"] --> S2["Debounce 500ms"]
    S2 --> S3["BumpVersion() sets version = timestamp"]
    S3 --> S4["Next system prompt build detects<br/>version change and reloads skills"]
```

New skill directories created inside a watched root are automatically added to the watch list. The debounce window (500ms) is shorter than the memory watcher (1500ms) because skill changes are lightweight.

---

## 11. Memory -- Indexing Pipeline

Memory documents are chunked, embedded, and stored for hybrid search.

```mermaid
flowchart TD
    IN["Document changed or created"] --> READ["Read content"]
    READ --> HASH["Compute SHA256 hash (first 16 bytes)"]
    HASH --> CHECK{"Hash changed?"}
    CHECK -->|No| SKIP["Skip -- content unchanged"]
    CHECK -->|Yes| DEL["Delete old chunks for this document"]
    DEL --> CHUNK["Split into chunks<br/>(max 1000 chars, prefer paragraph breaks)"]
    CHUNK --> EMBED{"EmbeddingProvider available?"}
    EMBED -->|Yes| API["Batch embed all chunks"]
    EMBED -->|No| SAVE
    API --> SAVE["Store chunks + tsvector index<br/>+ vector embeddings + metadata"]
```

### Chunking Rules

- Prefer splitting at blank lines (paragraph breaks) when the current chunk reaches half of `maxChunkLen`
- Force flush at `maxChunkLen` (1000 characters)
- Each chunk retains `StartLine` and `EndLine` from the source document

### Memory Paths

- `MEMORY.md` or `memory.md` at the workspace root
- `memory/*.md` (recursive, excluding `.git`, `node_modules`, etc.)

---

## 12. Hybrid Search

Combines full-text search and vector search with weighted merging.

```mermaid
flowchart TD
    Q["Search(query)"] --> FTS["FTS Search<br/>Standalone: SQLite FTS5 (BM25)<br/>Managed: tsvector + plainto_tsquery"]
    Q --> VEC["Vector Search<br/>Standalone: cosine similarity<br/>Managed: pgvector (cosine distance)"]
    FTS --> MERGE["hybridMerge()"]
    VEC --> MERGE
    MERGE --> NORM["Normalize FTS scores to 0..1<br/>Vector scores already in 0..1"]
    NORM --> WEIGHT["Weighted sum<br/>textWeight = 0.3<br/>vectorWeight = 0.7"]
    WEIGHT --> BOOST["Per-user scope: 1.2x boost<br/>Dedup: user copy wins over global"]
    BOOST --> RESULT["Sorted + filtered results"]
```

### Standalone vs Managed Comparison

| Aspect | Standalone | Managed |
|--------|-----------|---------|
| Storage | SQLite + FTS5 | PostgreSQL + tsvector + pgvector |
| FTS | `porter unicode61` tokenizer | `plainto_tsquery('simple')` |
| Vector | JSON array embedding | pgvector type |
| Scope | Global (single agent) | Per-agent + per-user |
| File watcher | fsnotify (1500ms debounce) | Not needed (DB-backed) |

When both FTS and vector search return results, scores are merged using the weighted sum. When only one channel returns results, its scores are used directly (weights normalized to 1.0).

---

## 13. Memory Flush -- Pre-Compaction

Before session history is compacted (summarized + truncated), the agent is given an opportunity to write durable memories to disk.

```mermaid
flowchart TD
    CHECK{"totalTokens >= threshold?<br/>(contextWindow - reserveFloor - softThreshold)<br/>AND not flushed in this cycle?"} -->|Yes| FLUSH
    CHECK -->|No| SKIP["Continue normal operation"]

    FLUSH["Memory Flush"] --> S1["Step 1: Build flush prompt<br/>asking to save memories to memory/YYYY-MM-DD.md"]
    S1 --> S2["Step 2: Provide tools<br/>(read_file, write_file, exec)"]
    S2 --> S3["Step 3: Run LLM loop<br/>(max 5 iterations, 90s timeout)"]
    S3 --> S4["Step 4: Mark flush done<br/>for this compaction cycle"]
    S4 --> COMPACT["Proceed with compaction<br/>(summarize + truncate history)"]
```

### Flush Defaults

| Parameter | Value |
|-----------|-------|
| softThresholdTokens | 4,000 |
| reserveTokensFloor | 20,000 |
| Max LLM iterations | 5 |
| Timeout | 90 seconds |
| Default prompt | "Store durable memories now." |

The flush is idempotent per compaction cycle -- it will not run again until the next compaction threshold is reached.

---

## File Reference

| File | Description |
|------|-------------|
| `internal/bootstrap/files.go` | Bootstrap file constants, loading, session filtering |
| `internal/bootstrap/truncate.go` | Truncation pipeline (head/tail split, budget clamping) |
| `internal/bootstrap/seed.go` | Standalone mode seeding (EnsureWorkspaceFiles) |
| `internal/bootstrap/seed_store.go` | Managed mode seeding (SeedToStore, SeedUserFiles) |
| `internal/bootstrap/load_store.go` | Load context files from DB (LoadFromStore) |
| `internal/bootstrap/templates/*.md` | Embedded template files |
| `internal/agent/systemprompt.go` | System prompt builder (BuildSystemPrompt, 15+ sections) |
| `internal/agent/memoryflush.go` | Memory flush logic (shouldRunMemoryFlush, runMemoryFlush) |
| `internal/skills/loader.go` | Skill loader (5-tier hierarchy, BuildSummary, filtering) |
| `internal/skills/search.go` | BM25 search index (tokenization, IDF scoring) |
| `internal/skills/watcher.go` | fsnotify watcher (500ms debounce, version bumping) |
| `internal/store/pg/skills.go` | Managed skill store (embedding search, backfill) |
| `internal/store/pg/skills_grants.go` | Skill grants (agent/user visibility, version pinning) |
| `internal/store/pg/memory_docs.go` | Memory document store (chunking, indexing, embedding) |
| `internal/store/pg/memory_search.go` | Hybrid search (FTS + vector merge, weighted scoring) |

---

## Cross-References

| Document | Relevant Content |
|----------|-----------------|
| [00-architecture-overview.md](./00-architecture-overview.md) | Startup sequence, managed mode wiring |
| [01-agent-loop.md](./01-agent-loop.md) | Agent loop calls BuildSystemPrompt, compaction flow |
| [03-tools-system.md](./03-tools-system.md) | ContextFileInterceptor routing read_file/write_file to DB |
| [06-store-data-model.md](./06-store-data-model.md) | memory_documents, memory_chunks tables |
