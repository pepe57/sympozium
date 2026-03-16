# Architecture Design: MCP Bridge Sidecar

**Version:** 1.0
**Date:** 2026-03-04
**Author:** BMad Master (BMAD Workflow)
**Status:** Draft

---

## 1. Architecture Overview

The MCP Bridge Sidecar is a new container added to Sympozium agent pods that translates between the existing file-based IPC protocol and remote MCP servers via JSON-RPC 2.0 Streamable HTTP transport. It runs alongside the existing `agent-runner`, `ipc-bridge`, and optional `sandbox`/`skill` sidecars.

### 1.1 System Context

```
┌──────────────────────────── Agent Pod ────────────────────────────┐
│                                                                   │
│  ┌───────────────┐                    ┌──────────────────┐        │
│  │  agent-runner  │───/ipc/output/───►│   ipc-bridge     │──►NATS │
│  │  (main)        │◄──/ipc/input/────│   (existing)      │◄──NATS │
│  │                │                   └──────────────────┘        │
│  │  Reads:        │                                               │
│  │  mcp-tools.json│   ┌──────────────────────────────────┐       │
│  │                │   │          mcp-bridge (NEW)          │       │
│  │  Writes:       │   │                                    │       │
│  │  mcp-request-* │──►│  fsnotify watcher                 │       │
│  │                │   │  ┌─────────────────────────┐      │       │
│  │  Reads:        │   │  │  Server Registry        │      │       │
│  │  mcp-result-*  │◄──│  │  ┌─────┐ ┌─────┐ ┌───┐ │      │       │
│  │                │   │  │  │srv-1│ │srv-2│ │...│ │      │       │
│  └───────────────┘   │  │  └──┬──┘ └──┬──┘ └─┬─┘ │      │       │
│                       │  └─────┼──────┼──────┼───┘      │       │
│  ┌───────────────┐   │        │      │      │           │       │
│  │  sandbox       │   └────────┼──────┼──────┼───────────┘       │
│  │  (optional)    │            │      │      │                    │
│  └───────────────┘            │      │      │                    │
│                                │      │      │                    │
└────────────────────────────────┼──────┼──────┼────────────────────┘
                                 │      │      │  Streamable HTTP
                    ┌────────────▼──────▼──────▼────────────────┐
                    │          K8s Services / External           │
                    │  ┌────────────────┐ ┌──────────────────┐  │
                    │  │mcp-k8s-network │ │otel-collector-mcp│  │
                    │  │:8080/mcp       │ │:8080/mcp         │  │
                    │  └────────────────┘ └──────────────────┘  │
                    │  ┌────────────────┐                        │
                    │  │mcp-proxy       │                        │
                    │  │:8080/mcp       │                        │
                    │  └────────────────┘                        │
                    └───────────────────────────────────────────┘
```

### 1.2 Component Responsibilities

| Component | Responsibility |
|-----------|---------------|
| **agent-runner** | Reads MCP tool manifest; dispatches MCP tool calls via IPC files; feeds results to LLM |
| **mcp-bridge** | Watches for MCP requests; routes to correct MCP server; translates JSON-RPC 2.0; writes results; discovers tools at startup |
| **ipc-bridge** | Unchanged — continues handling NATS relay for native tools, output, spawn, messages |
| **controller** | Builds mcp-bridge container when `mcpServers` is configured; mounts ConfigMap |

---

## 2. Detailed Component Design

### 2.1 MCP Bridge Sidecar (`cmd/mcp-bridge/`)

**Entry point:** `cmd/mcp-bridge/main.go`
**Core package:** `internal/mcpbridge/`

#### 2.1.1 Startup Sequence

```
1. Parse environment variables:
   - MCP_CONFIG_PATH (default: /config/mcp-servers.yaml)
   - MCP_IPC_PATH (default: /ipc/tools)
   - MCP_MANIFEST_PATH (default: /ipc/tools/mcp-tools.json)
   - OTEL_EXPORTER_OTLP_ENDPOINT (shared with agent pod)
   - OTEL_SERVICE_NAME (default: sympozium-mcp-bridge)
   - TRACEPARENT (W3C trace context from parent)
   - AGENT_RUN_ID (for logging/tracing correlation)
   - DEBUG (verbose logging)

2. Initialize OpenTelemetry (tracer + meter)

3. Load server registry from MCP_CONFIG_PATH

4. For each configured server:
   a. Establish Streamable HTTP session (POST to server URL)
   b. Call initialize (JSON-RPC: method="initialize")
   c. Call tools/list (JSON-RPC: method="tools/list")
   d. Store tool definitions with server prefix
   e. Log connection status + tool count
   f. On failure: log warning, skip server, continue

5. Build unified tool manifest → write to MCP_MANIFEST_PATH

6. Start fsnotify watcher on MCP_IPC_PATH for mcp-request-*.json files

7. Enter main loop: process requests until SIGTERM/SIGINT

8. On shutdown: close HTTP connections, flush traces, exit
```

#### 2.1.2 Server Registry Configuration

**File format:** `/config/mcp-servers.yaml` (mounted from ConfigMap)

```yaml
servers:
  - name: k8s-networking
    url: http://mcp-k8s-networking.tools.svc.cluster.local:8080/mcp
    transport: streamable-http
    toolsPrefix: "k8s_net"
    timeout: 30  # seconds
    auth:
      type: bearer  # "bearer" | "header"
      secretKey: MCP_K8S_NET_TOKEN  # env var name sourced from Secret
    headers:
      X-Custom-Header: value

  - name: otel-collector
    url: http://otel-collector-mcp.observability.svc.cluster.local:8080/mcp
    transport: streamable-http
    toolsPrefix: "otel"
    timeout: 60

  - name: mcp-proxy
    url: http://mcp-proxy.tools.svc.cluster.local:8080/mcp
    transport: streamable-http
    toolsPrefix: "proxy"
    timeout: 30
```

#### 2.1.3 Go Types

```go
// internal/mcpbridge/types.go

// ServerConfig defines a remote MCP server endpoint.
type ServerConfig struct {
    Name        string            `yaml:"name"`
    URL         string            `yaml:"url"`
    Transport   string            `yaml:"transport"`  // "streamable-http"
    ToolsPrefix string            `yaml:"toolsPrefix"`
    Timeout     int               `yaml:"timeout"`    // seconds, default 30
    Auth        *AuthConfig       `yaml:"auth,omitempty"`
    Headers     map[string]string `yaml:"headers,omitempty"`
}

type AuthConfig struct {
    Type      string `yaml:"type"`      // "bearer" or "header"
    SecretKey string `yaml:"secretKey"` // env var name with the token value
    HeaderName string `yaml:"headerName,omitempty"` // for type="header"
}

type ServersConfig struct {
    Servers []ServerConfig `yaml:"servers"`
}

// MCPRequest is written by agent-runner to /ipc/tools/mcp-request-<id>.json
type MCPRequest struct {
    ID        string            `json:"id"`
    Server    string            `json:"server"`    // server name or empty (resolved by prefix)
    Tool      string            `json:"tool"`      // prefixed tool name
    Arguments json.RawMessage   `json:"arguments"` // tool arguments
    Meta      map[string]string `json:"_meta,omitempty"` // trace context etc.
}

// MCPResult is written by mcp-bridge to /ipc/tools/mcp-result-<id>.json
type MCPResult struct {
    ID      string          `json:"id"`
    Success bool            `json:"success"`
    Content json.RawMessage `json:"content,omitempty"` // MCP tool result content
    Error   string          `json:"error,omitempty"`
    IsError bool            `json:"isError,omitempty"` // MCP-level error (tool returned error)
}

// MCPToolManifest is written to /ipc/tools/mcp-tools.json at startup
type MCPToolManifest struct {
    Tools []MCPToolDef `json:"tools"`
}

type MCPToolDef struct {
    Name        string         `json:"name"`        // prefixed name
    Description string         `json:"description"`
    Server      string         `json:"server"`      // server name for routing
    InputSchema map[string]any `json:"inputSchema"` // JSON Schema from MCP
}
```

#### 2.1.4 Request Processing Flow

```
1. fsnotify detects mcp-request-<id>.json in /ipc/tools/
2. Dedup check (sync.Map) — skip if already processing
3. Read and parse MCPRequest
4. Resolve target server:
   - If request.Server is set → use directly
   - Else → match request.Tool prefix against server registry
5. Strip tool prefix to get original MCP tool name
6. Build JSON-RPC 2.0 request:
   {
     "jsonrpc": "2.0",
     "id": <sequential>,
     "method": "tools/call",
     "params": {
       "name": "<original_tool_name>",
       "arguments": <request.Arguments>,
       "_meta": {
         "traceparent": "<W3C traceparent>"
       }
     }
   }
7. Create OTel span: "mcp.tools/call <server>/<tool>"
8. POST to server URL with headers + auth
9. Parse JSON-RPC 2.0 response
10. Build MCPResult from response
11. Write mcp-result-<id>.json
12. Close OTel span with status
13. Record metrics (duration, status, server, tool)
14. Remove processed file path from dedup map
```

### 2.2 Agent-Runner Changes (`cmd/agent-runner/`)

#### 2.2.1 MCP Tool Loading

Add to `main.go` startup, after skill loading:

```go
// Load MCP tools if manifest exists
mcpTools := loadMCPTools("/ipc/tools/mcp-tools.json")
if len(mcpTools) > 0 {
    tools = append(tools, mcpTools...)
    log.Printf("Loaded %d MCP tools from manifest", len(mcpTools))
}
```

#### 2.2.2 MCP Tool Dispatch

Add to `tools.go`:

```go
const mcpToolPrefix = "mcp__" // internal prefix to identify MCP tools in dispatch

func executeToolCall(ctx context.Context, name string, argsJSON string) string {
    // ... existing switch cases ...
    default:
        // Check if this is an MCP tool
        if mcpTool, ok := mcpToolRegistry[name]; ok {
            return executeMCPTool(ctx, mcpTool, argsJSON)
        }
        return fmt.Sprintf("Unknown tool: %s", name)
    }
}

func executeMCPTool(ctx context.Context, tool MCPToolDef, argsJSON string) string {
    id := fmt.Sprintf("%d", time.Now().UnixNano())

    req := MCPRequest{
        ID:        id,
        Server:    tool.Server,
        Tool:      tool.Name,
        Arguments: json.RawMessage(argsJSON),
        Meta:      traceMetadata(ctx),
    }

    toolsDir := "/ipc/tools"
    reqPath := filepath.Join(toolsDir, fmt.Sprintf("mcp-request-%s.json", id))
    resPath := filepath.Join(toolsDir, fmt.Sprintf("mcp-result-%s.json", id))

    // Write request, poll for result — mirrors executeCommand pattern
    data, _ := json.Marshal(req)
    os.WriteFile(reqPath, data, 0o644)

    // Poll with timeout (server-specific timeout + 10s buffer)
    deadline := time.Now().Add(time.Duration(tool.Timeout+10) * time.Second)
    for time.Now().Before(deadline) {
        resData, err := os.ReadFile(resPath)
        if err == nil && len(resData) > 0 {
            var result MCPResult
            if json.Unmarshal(resData, &result) == nil {
                os.Remove(reqPath)
                os.Remove(resPath)
                return formatMCPResult(result)
            }
        }
        time.Sleep(150 * time.Millisecond)
    }

    return "Error: timed out waiting for MCP tool result"
}
```

#### 2.2.3 Tool Name Strategy

MCP tool names presented to the LLM use the format: `{toolsPrefix}_{original_tool_name}`

Examples:
- Server `k8s-networking` with prefix `k8s_net`, tool `diagnose_gateway` → LLM sees `k8s_net_diagnose_gateway`
- Server `otel-collector` with prefix `otel`, tool `analyze_pipeline` → LLM sees `otel_analyze_pipeline`

The bridge strips the prefix when making the actual JSON-RPC call.

### 2.3 Controller Changes (`internal/controller/`)

#### 2.3.1 CRD Extension

Add to `SympoziumInstance` spec:

```go
// MCPServerRef defines an MCP server connection for agent pods.
type MCPServerRef struct {
    Name        string            `json:"name"`
    URL         string            `json:"url"`
    Transport   string            `json:"transport,omitempty"`   // default: "streamable-http"
    ToolsPrefix string            `json:"toolsPrefix"`
    Timeout     int               `json:"timeout,omitempty"`     // default: 30
    AuthSecret  string            `json:"authSecret,omitempty"`  // Secret name
    AuthKey     string            `json:"authKey,omitempty"`     // key within Secret
    Headers     map[string]string `json:"headers,omitempty"`
}

// In SympoziumInstanceSpec:
type SympoziumInstanceSpec struct {
    // ... existing fields ...
    MCPServers []MCPServerRef `json:"mcpServers,omitempty"`
}
```

#### 2.3.2 Pod Builder Extension

In `buildContainers()`, after the IPC bridge sidecar:

```go
// Add MCP bridge sidecar if MCP servers are configured
if len(mcpServers) > 0 {
    containers = append(containers, corev1.Container{
        Name:            "mcp-bridge",
        Image:           r.imageRef("mcp-bridge"),
        ImagePullPolicy: corev1.PullIfNotPresent,
        Env: []corev1.EnvVar{
            {Name: "MCP_CONFIG_PATH", Value: "/config/mcp-servers.yaml"},
            {Name: "MCP_IPC_PATH", Value: "/ipc/tools"},
            {Name: "AGENT_RUN_ID", Value: agentRun.Name},
            {Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: otelEndpoint},
            {Name: "OTEL_SERVICE_NAME", Value: "sympozium-mcp-bridge"},
            {Name: "TRACEPARENT", Value: traceparent},
        },
        VolumeMounts: []corev1.VolumeMount{
            {Name: "ipc", MountPath: "/ipc"},
            {Name: "mcp-config", MountPath: "/config", ReadOnly: true},
        },
        Resources: mcpBridgeResources(),
        SecurityContext: &corev1.SecurityContext{
            RunAsNonRoot:             ptr(true),
            ReadOnlyRootFilesystem:   ptr(true),
            AllowPrivilegeEscalation: ptr(false),
        },
    })
}
```

#### 2.3.3 ConfigMap Generation

Controller generates a ConfigMap from SympoziumInstance's `mcpServers` field:

```go
func (r *Reconciler) buildMCPConfigMap(instance *v1alpha1.SympoziumInstance) *corev1.ConfigMap {
    config := ServersConfig{Servers: convertMCPServerRefs(instance.Spec.MCPServers)}
    data, _ := yaml.Marshal(config)
    return &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-mcp-servers", instance.Name),
            Namespace: instance.Namespace,
        },
        Data: map[string]string{
            "mcp-servers.yaml": string(data),
        },
    }
}
```

### 2.4 Helm Chart Changes

#### 2.4.1 New Values

```yaml
# values.yaml additions
mcpBridge:
  enabled: false
  image:
    repository: sympozium/mcp-bridge
    tag: ""  # defaults to chart appVersion
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
  # Default MCP servers (can also be configured per-instance via CRD)
  servers: []
  # Example:
  # servers:
  #   - name: k8s-networking
  #     url: http://mcp-k8s-networking.tools.svc:8080/mcp
  #     toolsPrefix: k8s_net
  #     timeout: 30
```

### 2.5 Observability Design

#### 2.5.1 OTel Spans

| Span Name | Attributes | Parent |
|-----------|-----------|--------|
| `mcp.bridge.startup` | server_count, tool_count | root |
| `mcp.initialize` | server.name, server.url | startup |
| `mcp.tools/list` | server.name, tool_count | startup |
| `mcp.tools/call` | server.name, tool.name, tool.prefix | agent trace |
| `mcp.ipc.read_request` | request.id | tools/call |
| `mcp.ipc.write_result` | request.id, success | tools/call |

#### 2.5.2 Trace Propagation

```
Agent (TRACEPARENT env)
  → agent-runner creates span for tool call
    → writes traceparent to MCPRequest._meta
      → mcp-bridge extracts traceparent, creates child span
        → injects traceparent into JSON-RPC params._meta
          → MCP server (or mcp-proxy) continues trace
```

#### 2.5.3 Prometheus Metrics

```
# Counter
mcp_bridge_requests_total{server, tool, status}

# Histogram
mcp_bridge_request_duration_seconds{server, tool}

# Gauge
mcp_bridge_tools_registered{server}
mcp_bridge_server_connected{server}  # 1=connected, 0=disconnected
```

---

## 3. Architecture Decision Records (ADRs)

### ADR-001: File-Based IPC for Agent ↔ MCP Bridge Communication

**Status:** Accepted

**Context:**
The agent-runner communicates with sidecars via file-based IPC on the shared `/ipc` volume. Alternative approaches include direct HTTP calls from agent-runner to the bridge, or extending the NATS event bus.

**Decision:**
Use the existing file-based IPC pattern (`mcp-request-*.json` / `mcp-result-*.json` on `/ipc/tools/`).

**Rationale:**
- Consistent with existing `exec-request` / `exec-result` pattern for sandbox execution
- Minimal agent-runner changes (~80 lines of new code)
- No new network ports or service discovery needed within the pod
- Proven reliability with fsnotify + dedup pattern already in `internal/ipc/bridge.go`
- Security: agent-runner doesn't need HTTP client to external services

**Consequences:**
- ~150ms polling latency per tool call (matches existing pattern)
- File I/O overhead on tmpfs (negligible on ramdisk)
- MCP bridge must handle fsnotify dedup (pattern already established)

**Alternatives Rejected:**
- Direct HTTP from agent-runner: Breaks sidecar isolation; requires network policy changes
- NATS relay: Over-engineered; adds dependency on NATS for pod-internal communication
- Unix domain socket: Non-standard; would require custom protocol

---

### ADR-002: Streamable HTTP Transport for Bridge ↔ MCP Server Communication

**Status:** Accepted

**Context:**
MCP protocol supports multiple transports: stdio, SSE (legacy), and Streamable HTTP (current spec). The bridge needs to communicate with remote MCP servers over the network.

**Decision:**
Implement Streamable HTTP transport exclusively in v1.

**Rationale:**
- Streamable HTTP is the current MCP specification transport (2025-03-26)
- Works with standard HTTP infrastructure (load balancers, service mesh, TLS)
- Compatible with all three reference MCP servers
- Recommended by Henrik Rexed in issue #10 discussion
- Supports both request-response and streaming patterns

**Consequences:**
- MCP servers must support Streamable HTTP (most modern servers do)
- SSE-only legacy servers not supported in v1

**Alternatives Rejected:**
- SSE: Legacy transport being deprecated
- stdio: Not applicable for networked communication in K8s

---

### ADR-003: Dedicated Sidecar vs Extending Existing IPC Bridge

**Status:** Accepted

**Context:**
The MCP bridging functionality could be added to the existing `ipc-bridge` sidecar or implemented as a separate container.

**Decision:**
Create a new dedicated `mcp-bridge` sidecar container.

**Rationale:**
- **Single Responsibility:** IPC bridge handles NATS relay; MCP bridge handles MCP protocol translation
- **Independent lifecycle:** MCP bridge can be updated without affecting core IPC functionality
- **Optional deployment:** Only added to pods when `mcpServers` is configured
- **Independent resource limits:** MCP calls may need more memory/CPU than IPC relay
- **Testing isolation:** Can be tested independently with mock MCP servers

**Consequences:**
- Additional container in pod spec (3→4 base containers when enabled)
- Slightly more memory overhead (~128Mi)
- Shares the same `/ipc` volume

**Alternatives Rejected:**
- Extend ipc-bridge: Violates SRP; makes ipc-bridge deployment depend on MCP libraries
- Merge into agent-runner: Breaks isolation model; agent-runner shouldn't make external HTTP calls

---

### ADR-004: Tool Name Prefixing Strategy

**Status:** Accepted

**Context:**
Multiple MCP servers may expose tools with the same name (e.g., `list_resources`). The LLM needs unique tool names, and the bridge needs to route calls to the correct server.

**Decision:**
Use configurable `toolsPrefix` per server, applied as `{prefix}_{original_name}`.

**Rationale:**
- Prevents name collisions across servers
- Human-readable and LLM-friendly (e.g., `k8s_net_diagnose_gateway`)
- Bridge can deterministically route by stripping the prefix
- Operator controls naming via configuration

**Consequences:**
- Tool names are longer than original MCP names
- Prefix must be unique per server (validated at startup)
- Prefix is part of the tool's identity in SympoziumPolicy allow/deny lists

**Alternatives Rejected:**
- Server name as namespace (e.g., `k8s-networking::diagnose_gateway`): Special characters may confuse some LLMs
- Numeric prefix: Not human-readable
- No prefix (require globally unique tool names): Fragile; depends on MCP server authors

---

### ADR-005: Eager Tool Discovery at Startup

**Status:** Accepted

**Context:**
MCP tools can be discovered eagerly (at bridge startup) or lazily (on first call). The tool manifest must be available to the agent-runner before it starts the LLM conversation.

**Decision:**
Discover tools eagerly at startup and write a manifest file before the agent-runner begins.

**Rationale:**
- LLM needs the full tool list for its first API call
- Startup time is predictable (bounded by server count × timeout)
- Manifest file acts as readiness signal for agent-runner
- Failed servers are logged but don't block startup

**Consequences:**
- Bridge must start and complete discovery before agent-runner reads tools
- Pod startup ordering: mcp-bridge should write manifest quickly (within a few seconds)
- If MCP server is slow/down at startup, those tools are unavailable for the entire run

**Alternatives Rejected:**
- Lazy discovery: First tool call would be significantly slower; LLM doesn't know about tools upfront
- Init container: Would block pod startup entirely on MCP server availability
- Periodic re-discovery: Adds complexity; agent runs are short-lived (minutes, not hours)

---

### ADR-006: OTel Trace Context Propagation via MCP _meta Field

**Status:** Accepted

**Context:**
Sympozium already propagates W3C trace context via the `TRACEPARENT` environment variable and `_meta` field on `ExecRequest`. MCP protocol supports custom metadata in `params._meta`.

**Decision:**
Propagate W3C traceparent through the MCP `params._meta.traceparent` field, creating child spans in the bridge.

**Rationale:**
- Enables end-to-end trace visibility: Agent → Bridge → MCP Server
- Compatible with `mcp-proxy` which reads `_meta` for OTel context
- Follows existing Sympozium pattern for trace propagation
- MCP spec explicitly supports `_meta` for protocol-level metadata

**Consequences:**
- MCP servers must support `_meta` field (most do; those that don't simply ignore it)
- Bridge becomes a span producer (requires OTel SDK)
- Trace data volume increases proportionally with MCP call volume

---

### ADR-007: ConfigMap-Based Server Registry

**Status:** Accepted

**Context:**
MCP server endpoints and configuration need to be provided to the bridge sidecar. Options include: environment variables, ConfigMap mount, CRD field, or in-cluster service discovery.

**Decision:**
Use a ConfigMap (generated from SympoziumInstance CRD `mcpServers` field) mounted as a YAML file.

**Rationale:**
- Structured configuration with clear schema
- Generated from the CRD for declarative management
- ConfigMap can also be created independently for advanced use cases
- YAML format allows complex configuration (auth, headers, timeouts)
- Follows existing Sympozium patterns (skills, memory use ConfigMaps)

**Consequences:**
- Controller must generate/update ConfigMap when SympoziumInstance changes
- ConfigMap changes don't hot-reload (agent pods are ephemeral, so this is acceptable)
- Auth secrets are referenced by env var name, not stored in ConfigMap

---

## 4. Security Architecture

### 4.1 Network Access

```
┌─ Agent Pod ─────────────────────────────────┐
│                                              │
│  agent-runner ──X──► External (no egress)   │
│                                              │
│  mcp-bridge ──────► MCP servers (egress)    │
│                     - ClusterIP services     │
│                     - External HTTPS (opt)   │
│                                              │
│  ipc-bridge ──────► NATS (egress)           │
└──────────────────────────────────────────────┘
```

- Agent-runner has **no direct network access** to MCP servers
- MCP bridge has egress only to configured server endpoints
- NetworkPolicy restricts bridge egress to specific CIDRs/services

### 4.2 Credential Management

- Auth tokens stored in K8s Secrets, injected as environment variables
- ConfigMap references env var names, never credential values
- Bridge reads `os.Getenv(config.Auth.SecretKey)` at runtime

### 4.3 Container Security

```yaml
securityContext:
  runAsNonRoot: true
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: [ALL]
  seccompProfile:
    type: RuntimeDefault
```

### 4.4 Response Size Limits

- Maximum MCP response size: 1MB (configurable)
- Responses exceeding limit are truncated with warning
- Prevents memory exhaustion from malicious/broken MCP servers

---

## 5. Directory Structure (New Files)

```
cmd/
  mcp-bridge/
    main.go                  # Entry point, env parsing, signal handling

internal/
  mcpbridge/
    bridge.go                # Core bridge: watcher, request dispatcher
    config.go                # Server registry parsing
    client.go                # JSON-RPC 2.0 / MCP Streamable HTTP client
    manifest.go              # Tool discovery and manifest generation
    types.go                 # MCPRequest, MCPResult, MCPToolDef, etc.
    metrics.go               # Prometheus metrics registration
    bridge_test.go           # Unit tests
    client_test.go           # Client tests with mock MCP server

  ipc/
    protocol.go              # Add MCPRequest, MCPResult types (shared)

charts/
  sympozium/
    values.yaml              # Add mcpBridge section
    templates/               # Conditional mcp-bridge container in pod spec
```

---

## 6. Integration Points

### 6.1 With Existing IPC Bridge
- **No changes required.** The MCP bridge operates independently on the same `/ipc/tools/` directory.
- The existing IPC bridge ignores `mcp-request-*` and `mcp-result-*` files (it only watches for `exec-request-*` patterns).

### 6.2 With Agent-Runner Tool Framework
- New `loadMCPTools()` function reads manifest and creates `ToolDef` entries
- New default case in `executeToolCall()` checks `mcpToolRegistry` map
- New `executeMCPTool()` function mirrors `executeCommand()` IPC pattern

### 6.3 With SympoziumPolicy
- MCP tool names (with prefix) are subject to existing `toolPolicy` allow/deny lists
- Controller filters MCP tools against policy before generating the ConfigMap
- Example: `toolPolicy: {deny: ["k8s_net_delete_*"]}` blocks destructive k8s networking tools

### 6.4 With Observability Stack
- Bridge reuses the existing OTel initialization pattern from agent-runner
- Shares `OTEL_EXPORTER_OTLP_ENDPOINT` with other containers in the pod
- Trace context flows: TRACEPARENT → agent span → bridge span → MCP server span
