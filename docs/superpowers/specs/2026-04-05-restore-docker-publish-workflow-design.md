# Restore Docker Publish Workflow Design

## Goal

Restore the standalone GitHub Actions workflow `.github/workflows/docker-publish.yaml` that was deleted, so Docker images are again published on every push to `main` and via manual dispatch using the old exact behavior.

## Current State

The repository currently publishes Docker images through release-oriented workflows:

- `release.yaml` publishes versioned and latest images to GHCR and Docker Hub
- `release-beta.yaml` publishes beta-tagged images

The old standalone workflow `docker-publish.yaml` was deleted during the merge. That removed a separate deploy path that previously:

1. triggered on every push to `main`
2. allowed manual runs via `workflow_dispatch`
3. pushed Docker Hub images only
4. used the legacy image names:
   - `lbaominh/goclaw:latest`
   - `lbaominh/goclaw-ui:latest`
5. built only `linux/amd64`

## Requirements

### Functional requirements

1. Re-add `.github/workflows/docker-publish.yaml`
2. Keep the old exact trigger behavior:
   - `push` on `main`
   - `workflow_dispatch`
3. Keep the old exact image names and tags:
   - `lbaominh/goclaw:latest`
   - `lbaominh/goclaw-ui:latest`
4. Keep the old exact target platform:
   - `linux/amd64`
5. Use Docker Hub authentication via existing secrets:
   - `DOCKERHUB_USERNAME`
   - `DOCKERHUB_TOKEN`

### Non-functional requirements

1. Do not change `release.yaml`
2. Do not change `release-beta.yaml`
3. Keep the restored workflow behaviorally equivalent to the old deleted workflow
4. Minimize any repo-wide CI/CD changes outside the restored file

## Recommended Approach

Recreate the deleted workflow as a standalone file with the same runtime behavior, while keeping the newer release workflows untouched.

This avoids changing release semantics, satisfies the explicit request to add `docker-publish.yaml`, and restores the missing deploy path with the smallest possible change.

## Design

### Workflow file

Create or restore:

```text
.github/workflows/docker-publish.yaml
```

### Trigger model

The workflow should trigger on:

```yaml
on:
  push:
    branches:
      - main
  workflow_dispatch:
```

### Permissions

Keep permissions minimal:

```yaml
permissions:
  contents: read
```

### Environment variables

Use the exact old image targets:

```yaml
env:
  GOCLAW_IMAGE: lbaominh/goclaw:latest
  GOCLAW_UI_IMAGE: lbaominh/goclaw-ui:latest
```

### Jobs

Restore two independent jobs:

1. `build-and-push-goclaw`
2. `build-and-push-goclaw-ui`

Each job should:

1. checkout the repository
2. set up QEMU
3. set up Docker Buildx
4. log in to Docker Hub
5. build and push the image

### Backend image job

The backend job should preserve the old build behavior:

- context: `.`
- platforms: `linux/amd64`
- push: `true`
- tags: `${{ env.GOCLAW_IMAGE }}`
- build args:
  - `ENABLE_PYTHON=true`
  - `ENABLE_NODE=true`
  - `ENABLE_FULL_SKILLS=true`
- cache scope: `goclaw`

### Web UI image job

The web UI job should preserve the old build behavior:

- context: `ui/web`
- platforms: `linux/amd64`
- push: `true`
- tags: `${{ env.GOCLAW_UI_IMAGE }}`
- cache scope: `goclaw-ui`

## Interaction With Existing Release Workflows

The restored workflow intentionally coexists with:

- `.github/workflows/release.yaml`
- `.github/workflows/release-beta.yaml`

This means the repository will have:

1. release-based Docker publishing for versioned and current structured tags
2. legacy latest-only Docker publishing through `docker-publish.yaml`

That duplication is acceptable here because restoring the old exact behavior is the explicit requirement.

## Risks

1. Duplicate Docker publication paths may surprise future maintainers
2. The legacy workflow publishes only Docker Hub images, not GHCR
3. The workflow remains amd64-only even though newer release workflows are multi-arch

These are acceptable because they match the requested historical behavior.

## Testing

### Static verification

1. Confirm `.github/workflows/docker-publish.yaml` exists
2. Confirm the file contains the expected triggers, env vars, jobs, and Docker Hub login step

### Behavioral verification

1. Validate YAML structure with repo tooling if available
2. Inspect git diff to confirm only the workflow file is added for this change

## Out Of Scope

1. Refactoring the release workflows
2. Converting the restored workflow to GHCR
3. Converting the restored workflow to multi-arch
4. Renaming Docker images to the current release naming scheme

## Success Criteria

1. `.github/workflows/docker-publish.yaml` exists again
2. It triggers on push to `main` and manual dispatch
3. It pushes:
   - `lbaominh/goclaw:latest`
   - `lbaominh/goclaw-ui:latest`
4. It preserves the old standalone Docker deploy path without changing current release workflows
