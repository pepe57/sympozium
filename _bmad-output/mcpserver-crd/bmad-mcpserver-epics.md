# Epics & Stories — MCPServer CRD

## Epic 1: MCPServer CRD & Types (3 stories)

### Story 1.1 — Define MCPServer CRD Types
- Create `api/v1alpha1/mcpserver_types.go` with MCPServer, MCPServerSpec, MCPServerDeployment, MCPServerStatus
- Add kubebuilder markers: validation, defaults, printcolumns
- PrintColumns: NAME, TRANSPORT, READY, URL, TOOLS, AGE
- **Acceptance:** Types compile, markers valid

### Story 1.2 — Generate CRD Manifests
- Run `controller-gen` to generate deepcopy, CRD YAML, RBAC
- Add MCPServer to scheme registration
- Verify CRD installs cleanly: `kubectl apply -f config/crd/`
- **Acceptance:** CRD registers in cluster, `kubectl get mcpservers` works

### Story 1.3 — Add CRD to Helm Chart
- Add CRD YAML to `charts/sympozium/crds/`
- Add MCPServer examples to `charts/sympozium/examples/`
- **Acceptance:** `helm template` includes CRD, examples are valid YAML

---

## Epic 2: HTTP-to-Stdio Adapter (4 stories)

### Story 2.1 — Stdio Process Manager
- Create `internal/mcpbridge/stdio.go`
- `StdioManager` struct: spawn process, pipe stdin/stdout, restart on crash
- Exponential backoff on restart (1s, 2s, 4s, 8s, max 30s)
- Graceful shutdown: SIGTERM → wait 5s → SIGKILL
- Tests: spawn, crash+restart, graceful shutdown
- **Acceptance:** Process stays alive, restarts on crash, shuts down cleanly

### Story 2.2 — HTTP Server with JSON-RPC Translation
- Create `internal/mcpbridge/stdio_adapter.go`
- HTTP server on configurable port (default 8080)
- POST `/` → write JSON-RPC to stdin → read response from stdout → return HTTP
- POST `/mcp/v1/tools/list` → forward tools/list to stdio
- POST `/mcp/v1/tools/call` → forward tools/call to stdio
- Mutex for sequential stdin/stdout access
- Tests: HTTP → stdio round-trip, concurrent requests queued
- **Acceptance:** HTTP requests translate correctly to/from stdio

### Story 2.3 — OTel Instrumentation
- Wrap every request in `mcp.server.call` span
- Extract `traceparent` from HTTP header, create child span
- Span attributes: server, tool, transport, duration, status
- Metrics: `mcp.server.requests`, `mcp.server.errors`, `mcp.server.duration`
- Forward `_meta.traceparent` to stdio if present in JSON-RPC
- Tests: verify span creation, traceparent propagation
- **Acceptance:** Spans appear in collector, linked to parent trace

### Story 2.4 — Health Endpoint & CLI Integration
- `/healthz` returns 200 if stdio process alive, 503 if dead
- `/readyz` returns 200 if stdio process alive AND tools discovered
- Add `--stdio-adapter` flag to `cmd/mcp-bridge/main.go`
- Env var: `MCP_STDIO_ADAPTER=true` as alternative
- Configure via env: `STDIO_CMD`, `STDIO_ARGS`, `STDIO_PORT`, `STDIO_ENV_*`
- Tests: health check reflects process state
- **Acceptance:** Adapter starts via `mcp-bridge --stdio-adapter`, probes work

---

## Epic 3: MCPServer Controller (4 stories)

### Story 3.1 — Reconcile Loop Foundation
- Create `internal/controller/mcpserver_controller.go`
- Register with controller-runtime manager
- Basic reconcile: create/update/delete Deployments and Services
- Owner references for garbage collection
- Requeue on transient errors with backoff
- Tests: reconcile creates Deployment+Service, delete cleans up
- **Acceptance:** MCPServer CR → Deployment + Service created

### Story 3.2 — Stdio Mode: Build Pod Spec
- For `transportType: stdio`:
  - Single container: `mcp-bridge` image with `--stdio-adapter`
  - Env vars: `STDIO_CMD`, `STDIO_ARGS` from spec
  - Mount secrets from secretRefs as env vars
  - Readiness probe: `/readyz`, Liveness probe: `/healthz`
  - OTel env vars if observability enabled
- Tests: pod spec contains correct container, env, probes
- **Acceptance:** Stdio MCPServer produces correct pod spec

### Story 3.3 — HTTP Mode: Build Pod Spec
- For `transportType: http`:
  - Single container: user image directly
  - Port from spec (default 8080)
  - Mount secrets, env vars, serviceAccountName
  - Readiness/liveness probes on the HTTP port
- For `external`: skip Deployment creation, just validate URL
- Tests: HTTP mode pod spec, external mode no Deployment
- **Acceptance:** HTTP MCPServer deploys user image, external skips deployment

### Story 3.4 — Status Updates
- After Deployment ready: set `status.ready = true`, `status.url`
- Tool discovery: call `/mcp/v1/tools/list` on the Service, populate `status.toolCount` and `status.tools`
- Update conditions: Deployed, Ready, ToolsDiscovered
- Periodic re-check (every 5 min) for tool count changes
- Tests: status reflects deployment state, tools populated
- **Acceptance:** `kubectl get mcpservers` shows ready status, tool count, URL

---

## Epic 4: Agent Reference Resolution (2 stories)

### Story 4.1 — Resolve MCPServer Name → URL
- In `agentrun_controller.go`, when building MCP bridge config:
  - For each `mcpServers` entry without a `url`:
    - Look up MCPServer CR by name (check agent namespace first, then sympozium-system)
    - Read `status.url`
    - Inject into bridge config
  - If MCPServer not found or not ready: log warning, skip
- Tests: resolution finds MCPServer, injects URL
- **Acceptance:** Agent pod gets MCP bridge config with resolved URLs

### Story 4.2 — Backward Compatibility
- Existing inline `url` in MCPServerRef continues to work (no lookup needed)
- If both `url` and MCPServer CR exist, inline `url` takes precedence
- Existing agent runs unaffected
- Tests: inline URL bypasses resolution, mixed mode works
- **Acceptance:** No breaking changes for existing deployments

---

## Epic 5: Documentation & Examples (3 stories)

### Story 5.1 — MCPServer Deployment Guide
- `docs/mcp-servers.md`: overview of three modes (stdio, http, external)
- Step-by-step for each mode with complete YAML examples
- RBAC guide for HTTP MCP servers needing cluster access
- Troubleshooting section
- **Acceptance:** User can deploy all three modes following the guide

### Story 5.2 — SkillPack Creation Guide
- `docs/skillpacks.md`: how to create SkillPacks for MCP tool guidance
- Template structure, diagnostic methodology patterns
- Example: creating a SkillPack for a custom MCP server
- How to enable/disable SkillPacks per instance
- **Acceptance:** User can create and deploy a custom SkillPack

### Story 5.3 — Dynatrace MCP End-to-End Example
- Complete walkthrough: Dynatrace MCP server in Sympozium
- Secret creation for DT API token
- MCPServer CR with stdio transport
- SkillPack for Dynatrace diagnostics (DQL queries, log analysis)
- Verify agent can use Dynatrace tools
- **Acceptance:** Working Dynatrace MCP integration from scratch

---

## Dependency Graph

```
Epic 1 (CRD Types)
  ├── Epic 2 (Stdio Adapter)     ─┐
  └── Epic 3 (Controller)        ─┤
                                   ├── Epic 4 (Agent Resolution)
                                   └── Epic 5 (Documentation)
```

## Summary
- **5 epics, 16 stories**
- **Estimated effort:** ~5-7 days implementation
- **Key dependencies:** controller-runtime (existing), OTel SDK (existing)
- **No new external dependencies**
- **Backward compatible** — existing MCPServerRef with inline URL continues to work
