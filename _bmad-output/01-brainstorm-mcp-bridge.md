# MCP Bridge Sidecar — Brainstorming Session

**Date:** 2026-03-04
**Facilitator:** BMad Master
**Context:** GitHub Issue #10 — "Create a MCP bridge sidecar for easy MCP integration"

---

## 1. Problem Statement

Sympozium agents currently execute tools via file-based IPC through the existing IPC bridge sidecar, which relays commands to NATS and back. However, there is no mechanism for agents to invoke **external MCP (Model Context Protocol) servers** — standardized AI tool services exposing capabilities via JSON-RPC 2.0.

The MCP ecosystem is growing rapidly with servers like:
- **mcp-k8s-networking** — Gateway API, Istio, Cilium diagnostics
- **otel-collector-mcp** — OTel Collector pipeline debugging (12 analyzers)
- **mcp-proxy** — Universal sidecar adding OTel tracing to any MCP server

Sympozium needs a bridge that allows agents to transparently call remote MCP servers as if they were native tools.

---

## 2. Brainstorming Dimensions

### 2.1 Transport Protocol Options

| Option | Pros | Cons |
|--------|------|------|
| **A. File-based IPC (current pattern)** | Consistent with existing architecture; agent-runner already supports polling | Latency from filesystem polling; not portable outside K8s |
| **B. Streamable HTTP (MCP native)** | MCP spec-native; portable; simpler than fsnotify | Requires agent-runner changes to call HTTP directly; breaks current sidecar isolation |
| **C. Hybrid: File IPC → Bridge → Streamable HTTP** | Leverages existing IPC pattern for agent side; bridge handles MCP protocol translation | Extra hop; but maintains clean separation of concerns |
| **D. Direct HTTP from agent container** | Lowest latency; simplest path | Breaks sidecar isolation model; network policy complexity; harder to instrument |

**Winner: Option C (Hybrid)** — Agent writes `mcp-request-<id>.json` to `/ipc/tools/`, the MCP bridge sidecar picks it up, translates to JSON-RPC 2.0 Streamable HTTP, calls the remote MCP server, and writes `mcp-result-<id>.json` back. This preserves the existing IPC contract and keeps the agent-runner changes minimal (just new tool definitions that use the same file-based dispatch pattern).

### 2.2 Architecture Pattern Options

| Pattern | Description | Fit |
|---------|-------------|-----|
| **Sidecar per MCP server** | One bridge container per MCP endpoint | Over-provisioned; pod spec bloat |
| **Single bridge sidecar, multi-server** | One bridge container routing to N MCP servers via config | Clean; ConfigMap-driven; matches issue discussion |
| **Extend existing IPC bridge** | Add MCP capabilities to the current ipc-bridge | Violates single responsibility; complicates existing bridge |
| **Standalone gateway (DaemonSet/Deployment)** | Cluster-level MCP gateway | Adds network hop; harder multi-tenancy |

**Winner: Single bridge sidecar, multi-server** — One `mcp-bridge` container per agent pod, configured via ConfigMap with a server registry mapping tool prefixes to MCP server endpoints.

### 2.3 MCP Server Discovery & Configuration

**ConfigMap-based Server Registry:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mcp-servers-config
data:
  servers.yaml: |
    servers:
      - name: k8s-networking
        url: http://mcp-k8s-networking.tools.svc:8080/mcp
        transport: streamable-http
        tools_prefix: "k8s_net_"
      - name: otel-collector
        url: http://otel-collector-mcp.observability.svc:8080/mcp
        transport: streamable-http
        tools_prefix: "otel_"
      - name: custom-server
        url: https://external-mcp.example.com/mcp
        transport: streamable-http
        tools_prefix: "custom_"
        auth:
          type: bearer
          secretRef: custom-mcp-secret
```

**Key ideas:**
- Tool prefix routing: `k8s_net_diagnose_gateway` → routes to `k8s-networking` server, calls tool `diagnose_gateway`
- Instance-level config: SympoziumInstance CRD gets `mcpServers` field
- Hot-reload: Watch ConfigMap for changes without pod restart

### 2.4 Tool Discovery Flow

```
1. MCP Bridge starts → connects to all configured MCP servers
2. Bridge calls `tools/list` on each server (JSON-RPC 2.0)
3. Bridge builds unified tool registry with prefixed names
4. Bridge writes tool manifest to /ipc/tools/mcp-tools.json
5. Agent-runner reads manifest at startup → adds MCP tools to LLM tool list
6. When LLM calls an MCP tool → agent writes exec request → bridge routes to correct server
```

**Alternative: Lazy discovery** — Tools discovered on first call. Simpler but slower first invocation.

**Winner: Eager discovery at startup** — Bridge connects, lists tools, writes manifest. Agent reads manifest. This gives the LLM full tool awareness from the first turn.

### 2.5 Observability Integration

**OpenTelemetry Trace Propagation:**
- Agent-runner already has `TRACEPARENT` env var and OTel initialization
- MCP bridge should:
  1. Extract trace context from the exec request metadata
  2. Create child spans for each MCP call
  3. Inject W3C traceparent into MCP request `params._meta` field (per MCP spec)
  4. Record metrics: call duration, success/failure, server name, tool name

**Metrics to expose:**
- `mcp_bridge_request_total` (counter, labels: server, tool, status)
- `mcp_bridge_request_duration_seconds` (histogram, labels: server, tool)
- `mcp_bridge_tool_count` (gauge, labels: server)
- `mcp_bridge_connection_status` (gauge, labels: server)

**Integration with mcp-proxy:**
- mcp-proxy can sit between the bridge and any MCP server to add OTel tracing on the server side
- This creates end-to-end trace visibility: Agent → Bridge → mcp-proxy → MCP Server

### 2.6 Security Considerations

- **Network Policies:** Bridge needs egress to MCP server endpoints (ClusterIP services or external)
- **Authentication:** Support bearer tokens, mTLS, API keys via K8s Secrets
- **Input Validation:** Validate JSON-RPC requests before forwarding
- **Response Size Limits:** Prevent memory issues from large MCP responses (reuse 50KB limit pattern)
- **Timeout per server:** Configurable per-server call timeout
- **Policy Integration:** Respect SympoziumPolicy tool allow/deny lists for MCP tools

### 2.7 Error Handling & Resilience

- **Server unavailable:** Return structured error to agent; don't crash bridge
- **Timeout:** Per-server configurable timeout (default 30s)
- **Retry:** Optional retry with exponential backoff for transient failures
- **Circuit breaker:** If server fails N times, temporarily disable and report health
- **Graceful degradation:** Agent continues with non-MCP tools if bridge is unhealthy

### 2.8 Agent-Runner Integration Strategy

**Minimal changes to agent-runner:**
1. At startup, check for `/ipc/tools/mcp-tools.json` — if present, parse and add MCP tools to the tool list
2. MCP tool execution reuses existing `executeCommand`-style IPC pattern:
   - Write `/ipc/tools/mcp-request-<id>.json` with `{server, tool, arguments, trace_context}`
   - Poll for `/ipc/tools/mcp-result-<id>.json`
   - Parse and return result to LLM
3. New tool type `mcp_call` in the tool definitions that handles this dispatch

**This means:** Agent-runner needs ~50-100 lines of new code for MCP tool integration. The heavy lifting is in the bridge sidecar.

### 2.9 Deployment Topology

```
┌─────────────────── Agent Pod ───────────────────┐
│                                                  │
│  ┌──────────────┐  /ipc/  ┌──────────────────┐  │
│  │ agent-runner  │◄──────►│   ipc-bridge     │  │
│  │ (main)        │        │   (existing)      │  │
│  └──────┬───────┘        └────────┬──────────┘  │
│         │                         │              │
│         │  /ipc/tools/   ┌───────┴──────────┐   │
│         └───────────────►│   mcp-bridge     │   │
│                          │   (NEW sidecar)   │   │
│                          └───────┬──────────┘   │
│                                  │               │
│  ┌──────────────┐                │               │
│  │ sandbox      │                │               │
│  │ (optional)   │                │               │
│  └──────────────┘                │               │
└──────────────────────────────────┼───────────────┘
                                   │ HTTP/HTTPS
                    ┌──────────────┼──────────────┐
                    │              ▼              │
                    │  ┌──────────────────────┐  │
                    │  │ MCP Servers (K8s)     │  │
                    │  │ - mcp-k8s-networking  │  │
                    │  │ - otel-collector-mcp  │  │
                    │  │ - mcp-proxy           │  │
                    │  └──────────────────────┘  │
                    │                            │
                    │  K8s Cluster               │
                    └────────────────────────────┘
```

---

## 3. Key Design Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Transport: Agent ↔ Bridge | File-based IPC (existing pattern) | Minimal agent-runner changes; proven pattern |
| Transport: Bridge ↔ MCP Server | Streamable HTTP (JSON-RPC 2.0) | MCP spec-native; portable; recommended by contributors |
| Architecture | Dedicated sidecar, multi-server | Clean separation; doesn't bloat existing IPC bridge |
| Server config | ConfigMap + CRD field | Declarative; hot-reloadable; per-instance customization |
| Tool discovery | Eager at startup + manifest file | Full tool awareness from first LLM turn |
| Observability | OTel spans + metrics + `_meta` propagation | End-to-end tracing; compatible with mcp-proxy |
| Security | Secret-based auth + NetworkPolicy + Policy integration | Follows existing Sympozium security patterns |

---

## 4. Open Questions for PRD Phase

1. Should MCP tool results be cached? (e.g., `tools/list` results across agent runs)
2. Should the bridge support MCP `resources/` and `prompts/` in addition to `tools/`?
3. Should there be a cluster-wide MCP server registry vs. per-instance only?
4. What is the maximum number of MCP servers per agent pod?
5. Should MCP tools be subject to the same `toolPolicy` allow/deny as native tools?

---

## 5. Innovation Ideas

- **MCP Tool Marketplace:** Curated list of verified MCP servers deployable via Helm sub-charts
- **Auto-discovery:** Bridge queries K8s service annotations to find MCP servers automatically
- **Skill-to-MCP adapter:** Convert existing SkillPack sidecars to MCP servers for standardization
- **MCP Server health dashboard:** Real-time status of all connected MCP servers via API endpoint
- **Streaming tool results:** For long-running MCP operations, stream partial results back to agent
