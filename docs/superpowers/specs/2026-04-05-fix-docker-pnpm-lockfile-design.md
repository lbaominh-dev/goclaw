# Fix Docker PNPM Lockfile Build Failure Design

## Goal

Fix both Docker build paths that currently fail at `RUN pnpm install --frozen-lockfile` by repairing the broken web lockfile and aligning Docker's pnpm version with the version declared by the web workspace.

## Current State

Two Docker build paths currently fail:

1. Root Docker build:

```bash
docker build --target web-builder -f Dockerfile .
```

2. Standalone web Docker build:

```bash
docker build -f ui/web/Dockerfile ui/web
```

Both fail at:

```bash
pnpm install --frozen-lockfile
```

Observed error:

```text
ERR_PNPM_LOCKFILE_MISSING_DEPENDENCY
Broken lockfile: no entry for 'react-i18next@17.0.2(...)' in pnpm-lock.yaml
```

The error explicitly indicates a broken lockfile, likely from a bad merge resolution.

There is also pnpm version drift:

- `ui/web/package.json` declares `packageManager: pnpm@10.30.1`
- `ui/web/Dockerfile` uses `pnpm@10.30.1`
- root `Dockerfile` still uses `pnpm@10.28.2`

## Requirements

### Functional requirements

1. Repair `ui/web/pnpm-lock.yaml` so frozen installs succeed again
2. Align root `Dockerfile` to `pnpm@10.30.1`
3. Preserve `--frozen-lockfile` in both Docker build paths
4. Ensure both Docker builds complete past the install step

### Non-functional requirements

1. Keep the fix minimal and focused
2. Do not relax reproducibility by removing frozen-lockfile checks
3. Do not make unrelated dependency changes beyond what a clean pnpm 10 lockfile regeneration requires

## Recommended Approach

Regenerate the web lockfile using pnpm 10.30.1 and update the root `Dockerfile` to use the same pnpm version as the web workspace.

This fixes the actual broken artifact and removes version drift between the two Docker build entry points.

## Design

### 1. Lockfile repair

Regenerate:

```text
ui/web/pnpm-lock.yaml
```

using pnpm 10.30.1 so that the lockfile matches:

- `ui/web/package.json`
- `ui/web/.npmrc`
- current dependency graph expected by Docker builds

The repaired lockfile must include the previously missing dependency entry for the resolved `react-i18next@17.0.2(...)` package and all related package records required by the importer section.

### 2. Docker pnpm alignment

Update root `Dockerfile`:

From:

```dockerfile
RUN corepack enable && corepack prepare pnpm@10.28.2 --activate
```

To:

```dockerfile
RUN corepack enable && corepack prepare pnpm@10.30.1 --activate
```

Do not change `ui/web/Dockerfile` because it already uses `pnpm@10.30.1`.

### 3. Frozen install policy

Keep these lines unchanged in behavior:

```dockerfile
RUN pnpm install --frozen-lockfile
```

for both:

- root `Dockerfile`
- `ui/web/Dockerfile`

The fix is to make the lockfile valid again, not to weaken install validation.

### 4. Verification

Verify the repaired state with:

```bash
docker build --target web-builder -f Dockerfile .
docker build -f ui/web/Dockerfile ui/web
```

Both commands must complete the `pnpm install --frozen-lockfile` step successfully.

## Alternatives Considered

### Alternative 1: Fix only the lockfile

This would likely resolve the immediate error, but it would leave the root Dockerfile on an older pnpm pin than the workspace declares.

### Alternative 2: Remove `--frozen-lockfile`

Rejected because it hides the broken lockfile instead of fixing it and weakens reproducibility.

## Risks

1. Regenerated lockfile diff may be large
2. Regeneration must be done with pnpm 10, not pnpm 8, or the lockfile may be rewritten into an incompatible format

These are manageable and directly tied to the root cause.

## Testing

### Static verification

1. Confirm root `Dockerfile` now pins `pnpm@10.30.1`
2. Confirm `ui/web/Dockerfile` still uses `pnpm@10.30.1`
3. Confirm `ui/web/pnpm-lock.yaml` remains present and updated

### Build verification

1. `docker build --target web-builder -f Dockerfile .`
2. `docker build -f ui/web/Dockerfile ui/web`

## Out Of Scope

1. Changing dependency versions in `ui/web/package.json`
2. Converting Docker builds away from pnpm
3. Removing frozen lockfile enforcement
4. Refactoring the web dependency graph

## Success Criteria

1. Both Docker build paths pass the frozen pnpm install step
2. Root Dockerfile pnpm version matches the web workspace declaration
3. The lockfile is internally consistent again
