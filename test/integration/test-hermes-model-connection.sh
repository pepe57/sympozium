#!/usr/bin/env bash
# Real Hermes -> llama-server proof, including persistent state after pod restart.
set -euo pipefail
NAMESPACE="${TEST_NAMESPACE:-default}"
APISERVER_URL="${APISERVER_URL:-http://127.0.0.1:19091}"
APISERVER_NAMESPACE="${SYMPOZIUM_NAMESPACE:-sympozium-system}"
LLAMA_SERVER_URL="${LLAMA_SERVER_URL:?set the llama-server base URL, e.g. http://framework:8080/v1}"
NAME="${TEST_NAME:-hermes-framework-$(date +%s)}"
RUNTIME="${NAME}-runtime"
SESSION="${NAME}-chat"
TIMEOUT="${TEST_TIMEOUT:-300}"
EVIDENCE="${TEST_EVIDENCE_DIR:-/tmp/${NAME}-evidence}"
mkdir -p "$EVIDENCE"
APISERVER_TOKEN="${APISERVER_TOKEN:-}"
source "$(dirname "${BASH_SOURCE[0]}")/lib/resolve-token.sh"
PF_PID=""
cleanup() {
  if [[ "${KEEP_RESOURCES:-0}" != 1 ]]; then
    kubectl -n "$NAMESPACE" delete harnesssession "$SESSION" "${NAME}-yaml-chat" --ignore-not-found --wait=false >/dev/null 2>&1 || true
    kubectl -n "$NAMESPACE" delete agent "$NAME" "${NAME}-yaml" --ignore-not-found --wait=false >/dev/null 2>&1 || true
    kubectl -n "$NAMESPACE" delete modelconnection "$NAME" --ignore-not-found >/dev/null 2>&1 || true
    kubectl -n "$NAMESPACE" delete agentruntime "$RUNTIME" --ignore-not-found >/dev/null 2>&1 || true
  fi
  [[ -z "$PF_PID" ]] || kill "$PF_PID" 2>/dev/null || true
}
trap cleanup EXIT
api() {
  local method="$1" path="$2" file="${3:-}"
  local -a args=(--fail-with-body -sS --max-time "$TIMEOUT" -X "$method" -H 'Content-Type: application/json')
  [[ -z "$APISERVER_TOKEN" ]] || args+=(-H "Authorization: Bearer ${APISERVER_TOKEN}")
  [[ -z "$file" ]] || args+=(--data-binary "@$file")
  curl "${args[@]}" "${APISERVER_URL}${path}?namespace=${NAMESPACE}"
}
chat() {
  local session="$1" prompt="$2" output="$3"
  for _ in $(seq 1 10); do
    if api POST "/api/v1/harness-sessions/$session/chat" "$prompt" >"$output"; then return 0; fi
    # Retry only the bounded Service propagation window, not provider failures.
    [[ "$(cat "$output")" == "session adapter is unavailable" ]] || return 1
    sleep 2
  done
  return 1
}
wait_session() {
  local session="$1" end=$((SECONDS + TIMEOUT)) phase
  while (( SECONDS < end )); do
    phase="$(kubectl -n "$NAMESPACE" get harnesssession "$session" -o jsonpath='{.status.phase}')"
    [[ "$phase" != Ready ]] || return 0
    if [[ "$phase" == Failed ]]; then kubectl -n "$NAMESPACE" get harnesssession "$session" -o yaml; return 1; fi
    sleep 2
  done
  kubectl -n "$NAMESPACE" get harnesssession "$session" -o yaml
  return 1
}
if ! curl -fsS "$APISERVER_URL/healthz" >/dev/null 2>&1; then
  kubectl -n "$APISERVER_NAMESPACE" port-forward svc/sympozium-apiserver 19091:8080 >"$EVIDENCE/port-forward.log" 2>&1 &
  PF_PID=$!
  for _ in $(seq 1 30); do curl -fsS "$APISERVER_URL/healthz" >/dev/null 2>&1 && break; sleep 1; done
fi
resolve_apiserver_token

# Create an actual persistent Hermes runtime before connecting the model.
kubectl -n "$NAMESPACE" get agentruntime hermes-session-v0-20-6 -o json |
  jq --arg name "$RUNTIME" --arg ns "$NAMESPACE" '{apiVersion,kind,metadata:{name:$name,namespace:$ns},spec}' >"$EVIDENCE/runtime.json"
kubectl create -f "$EVIDENCE/runtime.json"
kubectl -n "$NAMESPACE" wait --for=condition=Ready "agentruntime/$RUNTIME" --timeout="${TIMEOUT}s"
curl --fail-with-body -sS --max-time 10 "${LLAMA_SERVER_URL%/}/models" >"$EVIDENCE/models.json"
MODEL="${TEST_MODEL:-$(jq -er '.data[0].id' "$EVIDENCE/models.json")}"
jq -n --arg name "$NAME" --arg endpoint "${LLAMA_SERVER_URL%/}/chat/completions" --arg model "$MODEL" '{name:$name,spec:{provider:"llama-server",protocol:"openai-chat",endpoint:$endpoint,models:[$model]}}' >"$EVIDENCE/connection-request.json"
api POST /api/v1/model-connections "$EVIDENCE/connection-request.json" >"$EVIDENCE/connection.json"
jq -n --arg name "$NAME" --arg model "$MODEL" --arg runtime "$RUNTIME" '{name:$name,model:$model,runtimeRef:$runtime,policyRef:"harness-examples",skills:[],channels:[],execution:{backend:"job",executionLifecycle:"one-shot",modelConnectionRef:$name}}' >"$EVIDENCE/agent-request.json"
api POST /api/v1/agents "$EVIDENCE/agent-request.json" >"$EVIDENCE/agent.json"
wait_session "$SESSION"
kubectl -n "$NAMESPACE" get deployment "$SESSION" -o json >"$EVIDENCE/deployment.json"
jq -e --arg model "$MODEL" --arg endpoint "${LLAMA_SERVER_URL%/}" '.spec.template.spec.containers[0].env | (any(.name=="MODEL_PROVIDER" and .value=="llama-server")) and (any(.name=="MODEL_NAME" and .value==$model)) and (any(.name=="MODEL_BASE_URL" and .value==$endpoint))' "$EVIDENCE/deployment.json" >/dev/null
TOKEN="framework-$RANDOM-$RANDOM"
jq -n --arg token "$TOKEN" '{messages:[{role:"user",content:("Remember this token: "+$token+". Reply with exactly HERMES-OK and the token. Do not use tools.")}]}'>"$EVIDENCE/prompt.json"
chat "$SESSION" "$EVIDENCE/prompt.json" "$EVIDENCE/first-response.json"
jq -e --arg token "$TOKEN" '.choices[0].message.content | contains("HERMES-OK") and contains($token)' "$EVIDENCE/first-response.json" >/dev/null
PVC_UID="$(kubectl -n "$NAMESPACE" get pvc "$SESSION" -o jsonpath='{.metadata.uid}')"
OLD_POD="$(kubectl -n "$NAMESPACE" get pods -l "app.kubernetes.io/instance=$SESSION" -o jsonpath='{.items[0].metadata.name}')"
kubectl -n "$NAMESPACE" delete pod "$OLD_POD" --wait=true
kubectl -n "$NAMESPACE" rollout status "deployment/$SESSION" --timeout="${TIMEOUT}s"
NEW_POD="$(kubectl -n "$NAMESPACE" get pods -l "app.kubernetes.io/instance=$SESSION" -o jsonpath='{.items[0].metadata.name}')"
[[ "$NEW_POD" != "$OLD_POD" ]]
[[ "$(kubectl -n "$NAMESPACE" get pvc "$SESSION" -o jsonpath='{.metadata.uid}')" == "$PVC_UID" ]]
printf '%s' '{"messages":[{"role":"user","content":"What token did I ask you to remember? Reply with the exact token only. Do not use tools."}]}' >"$EVIDENCE/recall-prompt.json"
chat "$SESSION" "$EVIDENCE/recall-prompt.json" "$EVIDENCE/recall-response.json"
jq -e --arg token "$TOKEN" '.choices[0].message.content | contains($token)' "$EVIDENCE/recall-response.json" >/dev/null

# Declarative creation uses the same connection and runtime, with no copied keys.
jq --arg name "${NAME}-yaml" --arg ns "$NAMESPACE" '{apiVersion:"sympozium.ai/v1alpha1",kind:"Agent",metadata:{name:$name,namespace:$ns},spec:{runtimeRef:.spec.runtimeRef,policyRef:.spec.policyRef,execution:.spec.execution,agents:{default:{model:.spec.agents.default.model}},memory:{enabled:false}}}' "$EVIDENCE/agent.json" >"$EVIDENCE/yaml-agent.json"
kubectl create -f "$EVIDENCE/yaml-agent.json"
jq -n --arg name "${NAME}-yaml-chat" --arg agent "${NAME}-yaml" --arg runtime "$RUNTIME" --arg ns "$NAMESPACE" '{apiVersion:"sympozium.ai/v1alpha1",kind:"HarnessSession",metadata:{name:$name,namespace:$ns},spec:{agentRef:$agent,runtimeRef:$runtime,desiredState:"running"}}' >"$EVIDENCE/yaml-session.json"
kubectl create -f "$EVIDENCE/yaml-session.json"
wait_session "${NAME}-yaml-chat"
chat "${NAME}-yaml-chat" "$EVIDENCE/prompt.json" "$EVIDENCE/yaml-response.json"
jq -e --arg token "$TOKEN" '.choices[0].message.content | contains($token)' "$EVIDENCE/yaml-response.json" >/dev/null
kubectl -n "$NAMESPACE" get harnesssession "$SESSION" "${NAME}-yaml-chat" -o json >"$EVIDENCE/sessions.json"
jq -n --arg runtime "$RUNTIME" --arg agent "$NAME" --arg connection "$NAME" --arg model "$MODEL" --arg oldPod "$OLD_POD" --arg newPod "$NEW_POD" --arg pvcUID "$PVC_UID" '{passed:true,runtime:$runtime,agent:$agent,connection:$connection,model:$model,oldPod:$oldPod,newPod:$newPod,pvcUID:$pvcUID,checks:["Hermes runtime Ready","connection creation","API Agent creation","model endpoint wiring","real prompt response","pod replacement","PVC retained","conversation recall after restart","declarative Agent and HarnessSession","declarative prompt response"]}' >"$EVIDENCE/result.json"
cat "$EVIDENCE/result.json"
echo "Evidence: $EVIDENCE"
