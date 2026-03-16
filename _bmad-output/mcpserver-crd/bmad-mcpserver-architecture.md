# Architecture — MCPServer CRD: Unified MCP Server Management

## ADR-001: Namespace-Scoped MCPServer

**Context:** MCPServer resources need to be discoverable by agent controllers across namespaces.

**Options:**
- A) Cluster-scoped — single global registry
- B) Namespace-scoped — deployed in sympozium-system, cross-namespace reference supported

**Decision:** **Namespace-scoped** (default: `sympozium-system`).
**Rationale:** Simpler RBAC, multi-tenant friendly. Agent controller can look up MCPServers in any namespace. Follows Kubernetes conventions (most CRDs are namespace-scoped).

---

## ADR-002: Built-in HTTP-to-Stdio Adapter

**Context:** Stdio MCP servers need an HTTP frontend for the MCP bridge to connect to.

**Options:**
- A) External tool like `supergateway` — adds dependency, different image
- B) Built into mcp-bridge binary as `mcp-bridge --stdio-adapter` mode
- C) Separate Go binary in the same image

**Decision:** **Option B — built into mcp-bridge binary.**
**Rationale:** No external dependency, single image for both bridge and adapter, full control over OTel instrumentation. Estimated ~200 lines of Go. Same image means no additional image pull.

**Implementation:**
```go
// cmd/mcp-bridge/main.go
if os.Getenv("MCP_STDIO_ADAPTER") == "true" || os.Args has "--stdio-adapter" {
    runStdioAdapter()  // HTTP server → spawn stdio process → pipe
    return
}
// ... existing bridge code
```

---

## ADR-003: Service URL Resolution

**Context:** Agent pods need to know the URL of MCPServer instances.

**Options:**
- A) Controller writes URL to MCPServer status, agent controller reads it
- B) Agent controller computes URL from naming convention
- C) ConfigMap with URL mappings

**Decision:** **Option A — status-based resolution.**
**Rationale:** Decoupled — MCPServer controller manages lifecycle and writes `status.url`. Agent controller reads the status when building pod spec. Handles all three modes (stdio/http/external) uniformly.

**Flow:**
```
MCPServer CR created
  → MCPServer controller reconciles
  → Creates Deployment + Service (if not external)
  → Writes status.url = http://<name>.<ns>.svc:8080
  → Agent controller reads status.url when building agent pod
  → Injects into MCP bridge config
```

---

## ADR-004: Stdio Process Management

**Context:** The adapter needs to manage the lifecycle of the stdio MCP server process.

**Decision:** Adapter spawns stdio process once at startup, keeps alive, restarts on crash with exponential backoff (1s, 2s, 4s, 8s, max 30s).

**Rationale:** MCP servers are typically stateless. Restart is cheap. Process-per-request would be too slow (~100ms+ startup per call).

**Implementation:**
- Child process stdin/stdout piped to adapter
- Adapter maintains a request queue → writes JSON-RPC to stdin → reads response from stdout
- Mutex for stdin/stdout access (stdio is sequential by nature)
- On process crash: drain pending requests with error, restart, reset backoff on success

---

## ADR-005: OTel Instrumentation on Adapter

**Context:** Need end-to-end traces through MCP server calls, including stdio servers that have no OTel support.

**Decision:** Adapter creates spans wrapping every stdio call. Uses standard OTel env vars (`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`) for export config.

**Span structure:**
```
mcp.server.call
  ├─ mcp.server: "dynatrace-mcp"
  ├─ mcp.tool: "query_logs"
  ├─ mcp.transport: "stdio"
  ├─ mcp.duration_ms: 245
  └─ mcp.status: "success"
```

**Metrics:**
- `mcp.server.requests` (counter, labels: server, tool, status)
- `mcp.server.errors` (counter, labels: server, tool, error_type)
- `mcp.server.duration` (histogram, labels: server, tool)

**Trace propagation:**
- Extracts `traceparent` from incoming HTTP request header
- Creates child span under that parent
- If stdio MCP server supports `_meta.traceparent` in JSON-RPC, forwards it

---

## Component Architecture

```
MCPServer CR (stdio)                    MCPServer CR (http)
  │                                       │
  ├─ MCPServer Controller                 ├─ MCPServer Controller
  │   │                                   │   │
  │   ├─ Deployment                       │   ├─ Deployment
  │   │   └─ Container: http-adapter      │   │   └─ Container: mcp-server
  │   │       ├─ mcp-bridge --stdio-adapter│  │       └─ (user image, HTTP)
  │   │       ├─ Spawns stdio process      │   │
  │   │       ├─ HTTP :8080                │   ├─ Service :8080
  │   │       ├─ OTel spans per call       │   │
  │   │       └─ /healthz                  │   └─ Status: url, ready, tools
  │   │                                    │
  │   ├─ Service :8080                     
  │   │                                    MCPServer CR (external)
  │   └─ Status:                             │
  │       ├─ url: http://name.ns.svc:8080    ├─ No deployment
  │       ├─ ready: true                     └─ Status:
  │       ├─ toolCount: 15                       └─ url: <user-provided>
  │       └─ tools: [query_logs, ...]
  │
  ▼
Agent Pod (existing architecture)
  ├─ Init: mcp-discover (reads MCPServer Service URLs)
  ├─ agent-runner (loads manifest, calls tools)
  ├─ mcp-bridge (HTTP → MCPServer Services)
  └─ ipc-bridge
```

## CRD Schema

```go
type MCPServer struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              MCPServerSpec   `json:"spec"`
    Status            MCPServerStatus `json:"status,omitempty"`
}

type MCPServerSpec struct {
    // TransportType: "stdio" or "http"
    TransportType string `json:"transportType"`

    // URL for external servers (no deployment)
    URL string `json:"url,omitempty"`

    // Deployment spec for managed servers
    Deployment *MCPServerDeployment `json:"deployment,omitempty"`

    // Common
    ToolsPrefix string `json:"toolsPrefix"`
    Timeout     int    `json:"timeout,omitempty"`    // default 30
    Replicas    *int32 `json:"replicas,omitempty"`   // default 1
}

type MCPServerDeployment struct {
    Image              string                       `json:"image"`
    Cmd                string                       `json:"cmd,omitempty"`
    Args               []string                     `json:"args,omitempty"`
    Port               int32                        `json:"port,omitempty"`  // default 8080
    Env                map[string]string            `json:"env,omitempty"`
    SecretRefs         []SecretRef                  `json:"secretRefs,omitempty"`
    Resources          corev1.ResourceRequirements  `json:"resources,omitempty"`
    ServiceAccountName string                       `json:"serviceAccountName,omitempty"`
}

type SecretRef struct {
    Name string `json:"name"`
}

type MCPServerStatus struct {
    Ready      bool              `json:"ready"`
    URL        string            `json:"url,omitempty"`
    ToolCount  int               `json:"toolCount,omitempty"`
    Tools      []string          `json:"tools,omitempty"`
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

## Key Dependencies
- `controller-runtime` (already used)
- OTel SDK (already in project)
- No new external dependencies
