# PRD — MCPServer CRD: Unified MCP Server Management

## Overview
Add a new MCPServer CRD to Sympozium that manages the full lifecycle of MCP servers in the cluster. Supports three deployment modes: stdio (with HTTP adapter), HTTP (deploy as-is), and external (URL reference only). One MCP server instance per cluster, shared by all agents.

## Functional Requirements

### FR-1: MCPServer Custom Resource Definition
- Namespace-scoped CRD (`sympozium.ai/v1alpha1`)
- Three transport modes: `stdio`, `http`, `external`
- For `stdio` and `http`: `deployment` spec with image, cmd, args, env, secretRefs, resources, serviceAccountName
- For `external`: `url` field, no deployment
- Common fields: `toolsPrefix`, `timeout`, `replicas` (default 1)

Example CRD usage:

```yaml
# Stdio MCP server — controller adds HTTP adapter
apiVersion: sympozium.ai/v1alpha1
kind: MCPServer
metadata:
  name: dynatrace-mcp
  namespace: sympozium-system
spec:
  transportType: stdio
  toolsPrefix: dt
  timeout: 30
  deployment:
    image: mcp/dynatrace-mcp-server:latest
    cmd: "node"
    args: ["/app/dist/index.js"]
    env:
      DT_GRAIL_QUERY_BUDGET_GB: "100"
      DT_MCP_DISABLE_TELEMETRY: "true"
    secretRefs:
      - name: dynatrace-mcp-secret
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 512Mi
---
# HTTP MCP server — controller deploys as-is
apiVersion: sympozium.ai/v1alpha1
kind: MCPServer
metadata:
  name: k8s-networking-mcp
  namespace: sympozium-system
spec:
  transportType: http
  toolsPrefix: k8s_net
  timeout: 30
  deployment:
    image: ghcr.io/henrikrexed/k8s-networking-mcp:latest
    port: 8080
    env:
      LOG_LEVEL: "info"
    serviceAccountName: k8s-networking-mcp
---
# External — no deployment, just register
apiVersion: sympozium.ai/v1alpha1
kind: MCPServer
metadata:
  name: external-mcp
  namespace: sympozium-system
spec:
  transportType: http
  toolsPrefix: ext
  url: http://existing-mcp.monitoring.svc:8080
```

### FR-2: Controller — Deployment Lifecycle
- Watches MCPServer resources
- For `stdio` servers: creates Deployment with HTTP-to-stdio adapter container + Service
- For `http` servers: creates Deployment with 1 container + Service
- For `external`: no Deployment, just validates URL reachable
- Handles updates (image change → rolling update), deletes (cleanup)
- Auto-creates Service: `<name>.<namespace>.svc:<port>`

### FR-3: HTTP-to-Stdio Adapter
- Built-in Go binary (ships in mcp-bridge image, invoked as `mcp-bridge --stdio-adapter`)
- Listens on HTTP port (default 8080)
- Spawns the stdio MCP server process as a child
- Translates HTTP JSON-RPC → stdin, stdout → HTTP response
- OTel instrumentation: creates `mcp.server.call` spans per request
- Propagates `traceparent` header → span context
- Health endpoint (`/healthz`) that checks stdio process is alive
- Graceful shutdown: sends SIGTERM to child, waits, then SIGKILL

### FR-4: Agent Reference Resolution
- SympoziumInstance `mcpServers` can reference by name (existing `name` field)
- Controller resolves MCPServer CR → Service URL from `status.url`
- Injects resolved URL into MCP bridge config automatically
- Falls back to existing behavior if `url` is provided directly (backward compatible)

### FR-5: Status & Observability
- `MCPServer.status`:
  - `ready: bool`
  - `url: string` (resolved Service URL)
  - `toolCount: int`
  - `tools: []string` (discovered tool names)
  - `conditions: []Condition` (Deployed, Ready, ToolsDiscovered)
- Adapter emits spans: `mcp.server.call` (tool, duration, status)
- Adapter emits metrics: `mcp.server.requests`, `mcp.server.errors`, `mcp.server.duration`

### FR-6: Security
- SecretRefs mounted as env vars (same pattern as current MCP bridge auth)
- ServiceAccountName for RBAC (admin provides, not auto-created)
- Adapter container runs as non-root, read-only filesystem
- Network policy: only agent pods can reach MCP server Services (optional)

### FR-7: Documentation
- How to deploy a stdio MCP server (Dynatrace example)
- How to deploy an HTTP MCP server (k8s-networking example)
- How to reference an external MCP server
- SkillPack creation guide for MCP tools
- Troubleshooting guide

## Non-Functional Requirements

### NFR-1: Backward Compatibility
- Existing `MCPServerRef` with inline `url` continues to work
- MCPServer CRD is additive — no breaking changes

### NFR-2: Performance
- Adapter adds < 5ms overhead per request
- Stdio process kept alive between calls (not spawned per-request)

### NFR-3: Reliability
- Adapter restarts stdio process on crash (with exponential backoff)
- Controller reconciles on drift (deleted Deployment/Service recreated)
- Readiness probe ensures traffic only reaches healthy MCP servers
