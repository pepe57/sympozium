# Epics & User Stories: MCP Bridge Sidecar

**Date:** 2026-03-04
**Author:** BMad Master (BMAD Workflow)
**GitHub Issue:** #10

---

## Epic 1: MCP Bridge Core Infrastructure

**Goal:** Build the foundational mcp-bridge sidecar binary with server registry, configuration parsing, and lifecycle management.

**Priority:** P0 — Must Have
**Estimated Stories:** 4

---

### Story 1.1: Create MCP Bridge Entry Point and Configuration

**As a** developer
**I want** a `cmd/mcp-bridge/main.go` entry point that loads configuration from a YAML file
**So that** the bridge can be started as a sidecar container with configurable MCP server connections.

**Acceptance Criteria:**
- [ ] New `cmd/mcp-bridge/main.go` with signal handling (SIGTERM/SIGINT)
- [ ] New `internal/mcpbridge/config.go` parsing `ServersConfig` from YAML
- [ ] Supports environment variables: `MCP_CONFIG_PATH`, `MCP_IPC_PATH`, `AGENT_RUN_ID`, `DEBUG`
- [ ] Validates config at startup: unique prefixes, valid URLs, non-empty names
- [ ] Structured JSON logging with `go-logr/logr`
- [ ] Exits cleanly on missing or empty config (no MCP servers = no-op exit)
- [ ] Unit tests for config parsing and validation

**Technical Notes:**
- Follow the same patterns as `cmd/ipc-bridge/main.go` for signal handling
- Use `gopkg.in/yaml.v3` for YAML parsing (already in go.mod)
- Config types defined in `internal/mcpbridge/types.go`

---

### Story 1.2: Implement JSON-RPC 2.0 MCP Client

**As a** developer
**I want** an MCP client that speaks JSON-RPC 2.0 over Streamable HTTP
**So that** the bridge can initialize sessions, list tools, and call tools on remote MCP servers.

**Acceptance Criteria:**
- [ ] New `internal/mcpbridge/client.go` implementing MCP Streamable HTTP client
- [ ] Supports `initialize` method (protocol version negotiation)
- [ ] Supports `tools/list` method (returns tool definitions)
- [ ] Supports `tools/call` method (invokes tool with arguments, returns result)
- [ ] Handles JSON-RPC 2.0 error responses gracefully
- [ ] Supports bearer token auth and custom headers
- [ ] Configurable per-server timeout (default 30s)
- [ ] Connection pooling via `http.Client` with transport settings
- [ ] Response size limit enforcement (default 1MB, configurable)
- [ ] Unit tests with httptest mock MCP server

**Technical Notes:**
- Use `net/http` standard library (no external JSON-RPC dependencies needed)
- JSON-RPC 2.0 request: `{"jsonrpc":"2.0","id":<int>,"method":"<method>","params":{...}}`
- MCP Streamable HTTP: POST to server URL, Content-Type: application/json, Accept: application/json
- Session management via `Mcp-Session-Id` response header

---

### Story 1.3: Implement Tool Discovery and Manifest Generation

**As a** developer
**I want** the bridge to discover tools from all configured MCP servers at startup and write a manifest
**So that** the agent-runner can load MCP tools into the LLM's tool list.

**Acceptance Criteria:**
- [ ] New `internal/mcpbridge/manifest.go`
- [ ] On startup, calls `initialize` then `tools/list` on each configured server
- [ ] Prefixes tool names with server's `toolsPrefix` + `_` separator
- [ ] Validates no prefix collisions across servers
- [ ] Writes `MCPToolManifest` JSON to `/ipc/tools/mcp-tools.json`
- [ ] If a server is unreachable, logs warning and continues with other servers
- [ ] If no tools discovered from any server, writes empty manifest (not an error)
- [ ] Manifest includes: prefixed name, description, server name, inputSchema
- [ ] Startup completes within 10s (with per-server timeout enforcement)
- [ ] Unit tests for prefix application and manifest generation

**Technical Notes:**
- MCP `tools/list` returns: `{tools: [{name, description, inputSchema}]}`
- Manifest format must match what `loadMCPTools()` in agent-runner expects
- Write atomically: write to temp file, then rename (prevent partial reads)

---

### Story 1.4: Implement Request Watcher and Dispatcher

**As a** developer
**I want** the bridge to watch for MCP request files, route them to the correct server, and write results
**So that** agent tool calls are processed end-to-end.

**Acceptance Criteria:**
- [ ] New `internal/mcpbridge/bridge.go` with fsnotify watcher on `/ipc/tools/`
- [ ] Watches for `mcp-request-*.json` file creation events
- [ ] Deduplicates events using `sync.Map` (matching IPC bridge pattern)
- [ ] Parses `MCPRequest`, resolves target server by tool prefix
- [ ] Strips prefix from tool name before calling MCP server
- [ ] Calls `tools/call` on the resolved server via the MCP client
- [ ] Writes `MCPResult` to `/ipc/tools/mcp-result-<id>.json`
- [ ] Handles errors: server not found, call failure, timeout → writes error result
- [ ] Cleans up request file after processing
- [ ] Concurrent request handling (goroutine per request, bounded by semaphore of 10)
- [ ] Graceful shutdown: drains in-flight requests before exit
- [ ] Unit tests with mock filesystem and mock MCP server

**Technical Notes:**
- Follow the same fsnotify + dedup pattern from `internal/ipc/bridge.go`
- Use `golang.org/x/sync/semaphore` for concurrency limiting
- Write result atomically (temp file + rename)

---

## Epic 2: Agent-Runner MCP Tool Integration

**Goal:** Enable the agent-runner to discover and invoke MCP tools alongside native tools.

**Priority:** P0 — Must Have
**Estimated Stories:** 2

---

### Story 2.1: Load MCP Tool Manifest in Agent-Runner

**As an** agent-runner
**I want to** read the MCP tool manifest and add MCP tools to my tool list
**So that** the LLM can see and select MCP tools alongside native tools.

**Acceptance Criteria:**
- [ ] New function `loadMCPTools(path string) []ToolDef` in `cmd/agent-runner/tools.go`
- [ ] Reads `/ipc/tools/mcp-tools.json` if it exists
- [ ] Waits up to 15s for manifest file to appear (bridge may still be starting)
- [ ] Converts `MCPToolDef` entries to `ToolDef` format for LLM tool list
- [ ] Builds internal `mcpToolRegistry` map for dispatch routing
- [ ] If manifest doesn't exist or is empty, proceeds with native tools only (no error)
- [ ] Logs count of loaded MCP tools
- [ ] Unit test with sample manifest file

**Technical Notes:**
- Wait logic: poll every 500ms for up to 15s. This is acceptable because agent-runner startup already takes 1-2s for skill loading.
- `ToolDef.Parameters` populated from MCP `inputSchema`
- Tool descriptions should include `[MCP: <server_name>]` suffix for LLM context

---

### Story 2.2: Implement MCP Tool Call Dispatch

**As an** agent-runner
**I want to** dispatch MCP tool calls via file-based IPC to the mcp-bridge
**So that** MCP tool invocations are executed on remote servers and results returned to the LLM.

**Acceptance Criteria:**
- [ ] New function `executeMCPTool(ctx, tool MCPToolDef, argsJSON string) string`
- [ ] Writes `MCPRequest` to `/ipc/tools/mcp-request-<id>.json`
- [ ] Polls for `/ipc/tools/mcp-result-<id>.json` with server timeout + 10s buffer
- [ ] Parses `MCPResult` and formats for LLM consumption
- [ ] Handles error results: returns structured error message
- [ ] Handles timeout: returns timeout error message
- [ ] Cleans up request and result files after processing
- [ ] Default dispatch in `executeToolCall()` switch: checks `mcpToolRegistry` before returning "Unknown tool"
- [ ] Trace metadata propagation via `MCPRequest._meta` field
- [ ] Unit tests for request writing, result parsing, timeout handling

**Technical Notes:**
- Mirror the exact pattern from `executeCommand()` (lines 622-698 in tools.go)
- Poll interval: 150ms (matching existing pattern)
- Format: success → return content as string; error → return `"MCP Error: <message>"`

---

## Epic 3: CRD and Controller Integration

**Goal:** Extend the Sympozium CRDs and controller to configure and deploy the MCP bridge sidecar.

**Priority:** P0 — Must Have
**Estimated Stories:** 3

---

### Story 3.1: Add MCPServers Field to SympoziumInstance CRD

**As a** platform operator
**I want to** configure MCP servers in the SympoziumInstance CRD
**So that** agents in that instance can access external MCP tools.

**Acceptance Criteria:**
- [ ] New `MCPServerRef` type in `api/v1alpha1/sympoziuminstance_types.go`
- [ ] `MCPServers []MCPServerRef` field added to `SympoziumInstanceSpec`
- [ ] Fields: name, url, toolsPrefix, timeout, authSecret, authKey, headers
- [ ] CRD validation: unique prefixes, valid URL format, non-empty name
- [ ] Generate updated CRD manifests (`make manifests`)
- [ ] Update CRD YAML in `config/crd/bases/`
- [ ] Sample CR in `config/samples/` demonstrating MCP server config

**Technical Notes:**
- Follow existing CRD patterns (e.g., `SkillRef`, `ChannelConfig`)
- Use kubebuilder markers for validation where applicable
- Run `controller-gen` to regenerate deepcopy and CRD YAML

---

### Story 3.2: Generate MCP ConfigMap in Controller

**As a** controller
**I want to** generate a ConfigMap from the SympoziumInstance's MCP server configuration
**So that** the mcp-bridge sidecar can read the server registry.

**Acceptance Criteria:**
- [ ] Controller creates/updates ConfigMap `<instance>-mcp-servers` when MCPServers is non-empty
- [ ] ConfigMap contains `mcp-servers.yaml` with the server registry
- [ ] Auth credentials are NOT stored in ConfigMap (only env var references)
- [ ] ConfigMap is owned by the SympoziumInstance (garbage collected on deletion)
- [ ] ConfigMap is created/updated during instance reconciliation
- [ ] If MCPServers becomes empty, ConfigMap is deleted
- [ ] Unit test for ConfigMap generation

**Technical Notes:**
- Use `controllerutil.SetControllerReference` for ownership
- Convert `MCPServerRef` → `ServerConfig` YAML format expected by bridge
- Auth secret → inject as env var from Secret in pod spec, reference env var name in ConfigMap

---

### Story 3.3: Add MCP Bridge Container to Agent Pod Builder

**As a** controller
**I want to** add the mcp-bridge sidecar container to agent pods when MCP servers are configured
**So that** agents can communicate with MCP servers.

**Acceptance Criteria:**
- [ ] `buildContainers()` conditionally adds `mcp-bridge` container when MCPServers is non-empty
- [ ] Container mounts: `/ipc` (shared), `/config` (ConfigMap, read-only)
- [ ] Environment variables: `MCP_CONFIG_PATH`, `AGENT_RUN_ID`, `OTEL_*`, `TRACEPARENT`
- [ ] Auth secrets injected as env vars from referenced K8s Secrets
- [ ] Security context: non-root, read-only FS, no privilege escalation
- [ ] Resource limits configurable (default: 128Mi memory, 100m CPU)
- [ ] `buildVolumes()` adds `mcp-config` volume (ConfigMap mount)
- [ ] Image ref follows existing pattern: `r.imageRef("mcp-bridge")`
- [ ] Unit test verifying pod spec with and without MCP bridge

**Technical Notes:**
- Follow the exact pattern of the ipc-bridge container addition (lines 810-860 in agentrun_controller.go)
- Pod has 3 containers minimum (agent + ipc-bridge + mcp-bridge) when MCP is enabled
- Sidecar cleanup: mcp-bridge should exit when `/ipc/output/result.json` appears (agent done)

---

## Epic 4: OpenTelemetry Instrumentation

**Goal:** Add end-to-end observability to MCP bridge operations.

**Priority:** P1 — Should Have
**Estimated Stories:** 2

---

### Story 4.1: Add OTel Tracing to MCP Bridge

**As a** platform operator
**I want** MCP tool calls to generate OTel trace spans
**So that** I can debug latency, failures, and trace requests end-to-end.

**Acceptance Criteria:**
- [ ] MCP bridge initializes OTel tracer provider (reuse agent-runner OTel init pattern)
- [ ] Each `tools/call` generates a span: `mcp.tools/call {server}/{tool}`
- [ ] Span attributes: `mcp.server.name`, `mcp.tool.name`, `mcp.tool.prefix`, `rpc.method`
- [ ] Span status set to error on failure
- [ ] W3C traceparent extracted from `MCPRequest._meta`
- [ ] W3C traceparent injected into MCP `params._meta.traceparent`
- [ ] Startup discovery generates spans: `mcp.initialize`, `mcp.tools/list`
- [ ] Traces exported to `OTEL_EXPORTER_OTLP_ENDPOINT`
- [ ] Graceful flush on shutdown

**Technical Notes:**
- Reuse OTel initialization from `cmd/agent-runner/main.go` (extract to shared package if beneficial)
- Use `go.opentelemetry.io/otel` SDK (already in go.mod)
- Parse traceparent with `go.opentelemetry.io/otel/propagation`

---

### Story 4.2: Add Prometheus Metrics to MCP Bridge

**As a** platform operator
**I want** MCP bridge metrics exposed for Prometheus scraping
**So that** I can monitor MCP tool call volume, latency, and error rates.

**Acceptance Criteria:**
- [ ] New `internal/mcpbridge/metrics.go` with metric registration
- [ ] Counter: `mcp_bridge_requests_total` (labels: server, tool, status)
- [ ] Histogram: `mcp_bridge_request_duration_seconds` (labels: server, tool)
- [ ] Gauge: `mcp_bridge_tools_registered` (labels: server)
- [ ] Gauge: `mcp_bridge_server_connected` (labels: server, 1/0)
- [ ] Metrics recorded on every tool call and server connection change
- [ ] Metrics exported via OTel metrics pipeline to OTLP endpoint

**Technical Notes:**
- Use OTel metrics API (`go.opentelemetry.io/otel/metric`)
- Metrics flow through OTel Collector → Prometheus (existing stack)
- No separate `/metrics` HTTP endpoint needed (bridge is ephemeral)

---

## Epic 5: Helm Chart and Deployment

**Goal:** Integrate the MCP bridge into the Sympozium Helm chart for declarative deployment.

**Priority:** P0 — Must Have
**Estimated Stories:** 2

---

### Story 5.1: Add MCP Bridge Configuration to Helm Values

**As a** platform operator
**I want to** configure the MCP bridge via Helm values
**So that** I can deploy it declaratively with `helm install/upgrade`.

**Acceptance Criteria:**
- [ ] `values.yaml`: new `mcpBridge` section with `enabled`, `image`, `resources`, `servers`
- [ ] Default: `mcpBridge.enabled: false`
- [ ] When enabled, controller is configured to add mcp-bridge sidecar
- [ ] Server list in values creates a default ConfigMap for all instances
- [ ] Example values documented with 3 reference MCP servers
- [ ] Schema validation in `values.schema.json` (if exists)

**Technical Notes:**
- Image follows existing pattern: `sympozium/mcp-bridge:<appVersion>`
- Resources default: requests 100m/128Mi, limits 200m/256Mi

---

### Story 5.2: Add MCP Bridge Container Image to CI/CD

**As a** developer
**I want** the mcp-bridge container image built and published alongside existing images
**So that** the Helm chart can reference it.

**Acceptance Criteria:**
- [ ] `Dockerfile` for mcp-bridge (multi-stage Go build)
- [ ] Added to existing CI/CD pipeline (GitHub Actions or equivalent)
- [ ] Image tagged with same version as other Sympozium components
- [ ] Image pushed to same registry as agent-runner, ipc-bridge, etc.
- [ ] Makefile target: `make build-mcp-bridge`, `make docker-mcp-bridge`

**Technical Notes:**
- Follow existing Dockerfile patterns (e.g., for ipc-bridge)
- Use `CGO_ENABLED=0` for static binary
- Base image: `gcr.io/distroless/static-debian12` or equivalent

---

## Epic 6: End-to-End Testing

**Goal:** Validate the MCP bridge works end-to-end with real and mock MCP servers.

**Priority:** P1 — Should Have
**Estimated Stories:** 2

---

### Story 6.1: Create Mock MCP Server for Testing

**As a** developer
**I want** a lightweight mock MCP server
**So that** I can test the bridge without external dependencies.

**Acceptance Criteria:**
- [ ] New `test/mock-mcp-server/` with a Go HTTP server implementing MCP protocol
- [ ] Supports `initialize`, `tools/list`, `tools/call` methods
- [ ] Configurable tool definitions and responses
- [ ] Returns structured errors for unknown tools
- [ ] Can simulate latency and failures
- [ ] Usable in both unit tests (httptest) and integration tests (container)

**Technical Notes:**
- Keep it minimal — just enough MCP protocol to validate the bridge
- Can also be used as a reference implementation for MCP server authors

---

### Story 6.2: Integration Tests with MCP Bridge

**As a** developer
**I want** integration tests that verify the full flow from agent request to MCP response
**So that** I can ensure the bridge works correctly in a pod-like environment.

**Acceptance Criteria:**
- [ ] Test: agent writes mcp-request → bridge processes → agent reads mcp-result
- [ ] Test: tool discovery writes correct manifest
- [ ] Test: multi-server routing (2+ mock servers with different prefixes)
- [ ] Test: server unavailable → error result (not crash)
- [ ] Test: timeout → error result
- [ ] Test: trace context propagated through the chain
- [ ] Tests run in CI pipeline
- [ ] Tests use temp directories to simulate `/ipc/tools/`

**Technical Notes:**
- Use `testing` package + `httptest` for mock servers
- Integration tests in `test/integration/mcpbridge/`
- Can run without K8s (file-based IPC is filesystem-only)

---

## Epic Summary

| Epic | Stories | Priority | Description |
|------|---------|----------|-------------|
| **E1: Core Infrastructure** | 1.1–1.4 | P0 | Bridge binary, config, client, watcher, dispatcher |
| **E2: Agent Integration** | 2.1–2.2 | P0 | Tool manifest loading, MCP tool dispatch |
| **E3: CRD & Controller** | 3.1–3.3 | P0 | CRD extension, ConfigMap gen, pod builder |
| **E4: Observability** | 4.1–4.2 | P1 | OTel traces, Prometheus metrics |
| **E5: Helm & Deployment** | 5.1–5.2 | P0 | Helm values, CI/CD image build |
| **E6: E2E Testing** | 6.1–6.2 | P1 | Mock MCP server, integration tests |

**Total Stories:** 15
**P0 (Must Have):** 11 stories
**P1 (Should Have):** 4 stories

---

## Suggested Implementation Order

```
Phase 1 (MVP):
  Story 1.1 → 1.2 → 1.3 → 1.4  (Core bridge, can test standalone)
  Story 2.1 → 2.2               (Agent integration)
  Story 6.1                      (Mock server for testing)

Phase 2 (K8s Integration):
  Story 3.1 → 3.2 → 3.3         (CRD + Controller)
  Story 5.1 → 5.2               (Helm + CI/CD)

Phase 3 (Production Readiness):
  Story 4.1 → 4.2               (Observability)
  Story 6.2                      (Integration tests)
```

---

## Dependencies Between Stories

```
1.1 ──► 1.2 ──► 1.3 ──► 1.4
                          │
2.1 ──► 2.2 ◄────────────┘
         │
3.1 ──► 3.2 ──► 3.3 ◄────┘
                  │
5.1 ──► 5.2 ◄────┘

6.1 ──► 6.2

4.1 ──► 4.2  (independent, can be done in parallel with Phase 2)
```
