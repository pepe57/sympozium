# Task: Prepare Sympozium OTel PR for upstream

## Context
We have a fork at `henrikrexed/sympozium` with OpenTelemetry instrumentation work. We need to prepare a clean PR branch against `AlexsJones/sympozium` upstream.

## Repo location
`/mnt/nas/project/sympozium/`

## What needs to happen

### 1. Rebase on upstream/main
- Upstream is at `974532c` (v0.0.79)
- Our OTel commits start at `5ea79af` (Epic 1) through `8bd9199`
- Rebase our OTel work on top of upstream/main
- Resolve any conflicts

### 2. Filter commits for the PR
**INCLUDE** (core OTel instrumentation):
- `5ea79af` feat(otel): Epic 1 - add pkg/telemetry package + BMAD docs
- `68fbfea` feat(otel): Epic 2 - controller OTel instrumentation + CRD ObservabilitySpec
- `1df1f0e` feat(otel): Epic 3 - agent-runner OTel instrumentation
- `5c1d0e9` feat(otel): Epic 4 - API server OTel instrumentation
- `88d6bfc` feat(otel): Epic 5 - metrics instrumentation
- `049e150` feat(otel): Epic 6 - structured logging with trace correlation
- `4ec854d` feat(otel): Epic 7 - configuration, Helm, and testing
- `e0b6c55` feat(otel): end-to-end trace propagation across channel → NATS → controller → agent
- `8bd9199` feat(otel): inject OTel env vars into channel pods + use configurable image registry

**EXCLUDE** (fork-specific, CI, debug):
- `3ff56a9` ci: build Docker images to ghcr.io/henrikrexed/sympozium
- `6ec6ed0` fix(helm): repair apiserver deployment template
- `5bd1336` ci: add channel images and skill-k8s-ops to Docker build matrix
- `3de918f` fix(slack): add ping/pong handlers and increase WebSocket read deadline
- `c72742c` debug: add WebSocket message logging to slack channel
- `72bcf15` feat: make image registry configurable via SYMPOZIUM_IMAGE_REGISTRY env var

**SPECIAL HANDLING**:
- The image registry configurability from `72bcf15` and `8bd9199` should be included BUT using upstream's image registry as default, not hardcoding henrikrexed
- The apiserver template fix from `6ec6ed0` may already be fixed upstream — check
- The slack ping/pong fix from `3de918f` is a real bug fix — include it as a separate commit or note it

### 3. Create PR branch
- Branch name: `feat/otel-instrumentation`
- Base: upstream/main
- Clean commit history (squash into logical commits if needed, or keep the 7 epics + 2 extras)

### 4. Remove fork-specific references
- Remove any `ghcr.io/henrikrexed/` hardcoded references
- Remove BMAD docs from `_bmad/` directory (too large for PR, reference in PR description)
- Keep `image.registry` in Helm values as the upstream default (`ghcr.io/alexsjones/sympozium`)

### 5. Verify build
- `go build ./...` must pass
- `go vet ./...` must pass
- No lint errors

### 6. Write PR description
Create a file `pr-description.md` with:
- Title: "feat: OpenTelemetry instrumentation for end-to-end agent observability"
- Reference issue #11
- Summary of changes per epic
- Configuration example (SympoziumInstance CR with observability)
- What traces look like (span names, attributes)
- Screenshots note (we have Dynatrace traces working)
- Testing done

## Important
- Do NOT push to upstream — just create the branch on our fork
- The upstream default image registry should remain `ghcr.io/alexsjones/sympozium`
- Keep the `SYMPOZIUM_IMAGE_REGISTRY` env var support (it's useful for any fork)
