# Product Requirements Document: MCP Bridge Sidecar

**Version:** 1.0
**Date:** 2026-03-04
**Author:** BMad Master (BMAD Workflow)
**Status:** Draft
**GitHub Issue:** #10 — "Create a MCP bridge sidecar for easy MCP integration"

---

## 1. Executive Summary

This PRD defines the requirements for an MCP (Model Context Protocol) Bridge Sidecar that enables Sympozium agents to invoke tools exposed by remote MCP servers via the JSON-RPC 2.0 protocol. The bridge runs as an additional sidecar container in agent pods, translating between the existing file-based IPC mechanism and MCP's Streamable HTTP transport. It supports multiple MCP servers per agent, ConfigMap-driven configuration, and end-to-end OpenTelemetry observability.

---

## 2. Problem Statement

### Current State
Sympozium agents can execute built-in tools (file operations, URL fetch, command execution, channel messaging) through native handlers or IPC-mediated sidecar execution. There is no mechanism to call external tool servers using the Model Context Protocol standard.

### Desired State
Agents can transparently call any MCP-compliant server (inside or outside the K8s cluster) as if the MCP tools were native Sympozium tools. The integration preserves existing security, observability, and isolation guarantees.

### Impact
- Unlocks the entire MCP tool ecosystem for Sympozium agents
- Enables specialized K8s operations (networking diagnostics, OTel debugging) without custom tool code
- Positions Sympozium as an MCP-native orchestration platform

---

## 3. User Personas

| Persona | Description | Need |
|---------|-------------|------|
| **Platform Operator** | Deploys Sympozium, manages instances | Configure MCP server endpoints; monitor bridge health; control access |
| **Agent Developer** | Designs agent workflows and prompts | Discover available MCP tools; use them in agent tasks; debug failures |
| **MCP Server Author** | Builds MCP-compliant tool servers | Deploy server in K8s; register with Sympozium; validate integration |

---

## 4. User Stories

### US-1: Configure MCP Servers
**As a** Platform Operator
**I want to** define MCP server endpoints in a ConfigMap or SympoziumInstance CRD
**So that** agents in that instance can access the configured MCP tools

**Acceptance Criteria:**
- Server config includes: name, URL, transport type, auth reference, tool prefix
- Changes to ConfigMap are picked up without pod restart (for long-lived scenarios) or at pod creation
- Invalid config is rejected with clear error messages

### US-2: Discover MCP Tools
**As an** Agent Developer
**I want** MCP tools to appear in the agent's tool list alongside native tools
**So that** the LLM can select and invoke them naturally

**Acceptance Criteria:**
- MCP bridge calls `tools/list` on each configured server at startup
- Tools are prefixed with the server's `tools_prefix` to avoid name collisions
- Tool descriptions and parameter schemas from MCP are preserved in the tool definitions
- If a server is unreachable at startup, the bridge logs a warning and continues with available servers

### US-3: Execute MCP Tool Calls
**As an** Agent (LLM)
**I want to** call an MCP tool by name with JSON arguments
**So that** I can perform operations on remote systems

**Acceptance Criteria:**
- Agent writes an MCP request file to `/ipc/tools/mcp-request-<id>.json`
- Bridge routes the request to the correct MCP server based on tool prefix
- Bridge sends a `tools/call` JSON-RPC 2.0 request via Streamable HTTP
- Bridge writes the result to `/ipc/tools/mcp-result-<id>.json`
- Agent reads the result and returns it to the LLM loop
- Timeout is enforced per-server (configurable, default 30s)

### US-4: End-to-End Observability
**As a** Platform Operator
**I want** MCP tool calls to be traced end-to-end
**So that** I can debug latency, failures, and usage patterns

**Acceptance Criteria:**
- Bridge creates OTel spans for each MCP call with attributes: server name, tool name, status
- W3C traceparent is propagated via MCP `params._meta` field
- Prometheus metrics are exposed: request count, duration histogram, error rate
- Trace IDs are included in error messages for correlation

### US-5: Secure MCP Communication
**As a** Platform Operator
**I want** MCP server authentication to use K8s Secrets
**So that** credentials are never stored in ConfigMaps or environment variables

**Acceptance Criteria:**
- Auth types supported: bearer token, API key header
- Credentials sourced from referenced K8s Secret
- Network policies allow bridge egress only to configured MCP endpoints
- Response size limits prevent memory exhaustion (configurable, default 1MB)

### US-6: Helm Chart Deployment
**As a** Platform Operator
**I want to** enable the MCP bridge via Helm values
**So that** I can deploy it declaratively alongside the existing stack

**Acceptance Criteria:**
- `values.yaml` includes `mcpBridge.enabled` toggle (default: false)
- MCP server configuration is defined in values or external ConfigMap reference
- Bridge container image, resources, and security context are configurable
- Documentation includes example values for the 3 reference MCP servers

### US-7: Policy Integration
**As a** Platform Operator
**I want** MCP tools to respect SympoziumPolicy tool allow/deny lists
**So that** I can control which MCP tools agents can invoke

**Acceptance Criteria:**
- MCP tool names (with prefix) are subject to the same `toolPolicy` as native tools
- Denied MCP tools are excluded from the tool manifest provided to agents
- Policy violations are logged and reported

---

## 5. Functional Requirements

### FR-1: MCP Bridge Container
- Go binary running as a sidecar in agent pods
- Shares `/ipc` volume with agent-runner and ipc-bridge
- Watches `/ipc/tools/` for `mcp-request-*.json` files using fsnotify
- Implements JSON-RPC 2.0 client for MCP Streamable HTTP transport
- Writes results to `/ipc/tools/mcp-result-*.json`

### FR-2: Multi-Server Routing
- Parses server registry from mounted ConfigMap (`/config/mcp-servers.yaml`)
- Routes requests by matching tool name prefix to server configuration
- Maintains persistent HTTP connections to MCP servers (connection pooling)

### FR-3: Tool Discovery & Manifest
- On startup, calls `tools/list` on each configured MCP server
- Builds unified tool manifest with prefixed tool names
- Writes manifest to `/ipc/tools/mcp-tools.json`
- Manifest format matches agent-runner's `ToolDef` structure

### FR-4: Agent-Runner Integration
- Agent-runner reads `/ipc/tools/mcp-tools.json` at startup
- Adds MCP tools to the LLM tool list
- New `mcp_call` dispatch handler writes request files and polls for results
- Reuses existing IPC polling pattern with configurable timeout

### FR-5: CRD Integration
- SympoziumInstance spec gains `mcpServers` field for inline server config
- AgentRun status gains `mcpToolCalls` counter for tracking
- Controller builds MCP bridge sidecar when `mcpServers` is non-empty

### FR-6: Health & Readiness
- Bridge exposes health endpoint (file-based or HTTP on localhost)
- Reports per-server connection status
- Agent-runner waits for MCP tool manifest before starting LLM loop

---

## 6. Non-Functional Requirements

### NFR-1: Performance
- MCP tool call overhead (bridge processing) < 10ms excluding network RTT
- Tool manifest generation < 2s for up to 10 servers with 50 tools each
- File-based IPC polling interval: 100ms (matching existing pattern)

### NFR-2: Reliability
- Bridge continues operating if individual MCP servers are unavailable
- No single MCP server failure crashes the bridge or agent pod
- Graceful shutdown: drain in-flight requests on SIGTERM

### NFR-3: Security
- Bridge runs as non-root with read-only root filesystem
- Minimal RBAC: only needs ConfigMap and Secret read access
- No direct network access from agent-runner to MCP servers

### NFR-4: Resource Limits
- Default: 128Mi memory, 100m CPU (configurable via Helm)
- Response size limit: 1MB per MCP call (configurable)
- Maximum concurrent MCP calls: 10 per bridge instance

### NFR-5: Observability
- Structured JSON logging with correlation IDs
- OTel traces for every MCP interaction
- Prometheus metrics endpoint on localhost

### NFR-6: Compatibility
- MCP protocol version: 2025-03-26 (Streamable HTTP)
- Go 1.25+ (matching project Go version)
- Kubernetes 1.28+ (matching Helm chart requirements)

---

## 7. Out of Scope (v1)

- MCP `resources/` and `prompts/` capabilities (tools only in v1)
- SSE (Server-Sent Events) transport (Streamable HTTP only)
- stdio transport (not applicable in K8s sidecar model)
- MCP server auto-discovery via K8s service annotations
- MCP tool result caching
- Cluster-wide MCP gateway (per-pod sidecar only)
- Bidirectional MCP (Sympozium as MCP server)

---

## 8. Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| MCP tool call success rate | > 99% (excluding server-side errors) | Prometheus counter |
| Bridge startup time | < 5s including tool discovery | OTel span |
| Tool manifest accuracy | 100% (all server tools listed) | Integration test |
| Memory overhead per bridge | < 64MB steady state | K8s metrics |
| E2E trace completion | 100% of MCP calls have traces | Jaeger/Tempo query |

---

## 9. Dependencies

| Dependency | Type | Status |
|------------|------|--------|
| Existing IPC bridge pattern | Internal | Available — proven in production |
| fsnotify library | External | Already used in ipc-bridge |
| MCP Go SDK or JSON-RPC 2.0 client | External | Needs evaluation (or custom implementation) |
| mcp-k8s-networking server | External | Available — open source |
| otel-collector-mcp server | External | Available — open source |
| mcp-proxy server | External | Available — open source |
| OTel Go SDK | External | Already integrated in agent-runner |

---

## 10. Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| MCP spec evolves breaking changes | High | Medium | Pin to specific MCP version; abstract protocol layer |
| MCP server response too large | Medium | Medium | Enforce response size limits; truncate with warning |
| Network latency to external MCP servers | Medium | High | Per-server timeouts; async execution model |
| Tool name collisions across servers | Low | Low | Mandatory unique prefix per server |
| Go MCP SDK immaturity | Medium | Medium | Evaluate `github.com/mark3labs/mcp-go`; fallback to custom JSON-RPC client |

---

## 11. Release Plan

| Phase | Scope | Target |
|-------|-------|--------|
| **v0.1** | Core bridge + single server + agent integration | MVP |
| **v0.2** | Multi-server routing + ConfigMap config + CRD integration | Beta |
| **v0.3** | Full OTel instrumentation + metrics + health checks | GA-ready |
| **v0.4** | Helm chart integration + documentation + E2E tests | GA |
