import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, getNamespace, type ModelConnection } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

export function NativeModelSelector({ connectionRef, provider, model, onChange, native = true }: {
  native?: boolean; connectionRef?: string; provider: string; model: string;
  onChange: (value: { modelConnectionRef?: string; provider: string; model: string }) => void;
}) {
  const queryClient = useQueryClient();
  const namespace = getNamespace();
  const connections = useQuery({ queryKey: ["model-connections", namespace], queryFn: api.modelConnections.list });
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState({ name: "", provider: "", protocol: "openai-chat" as ModelConnection["spec"]["protocol"], endpoint: "", credentialProfile: "", secretRef: "", models: "" });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const selected = connections.data?.find((connection) => connection.metadata.name === connectionRef);
  async function save() {
    setError("");
    setSaving(true);
    try {
      const connection = await api.modelConnections.create({ name: draft.name, spec: { provider: draft.provider, protocol: draft.protocol, endpoint: draft.endpoint, credentialProfile: native ? draft.credentialProfile : undefined, secretRef: native ? undefined : draft.secretRef || undefined, models: draft.models.split(",").map((s) => s.trim()).filter(Boolean) } });
      await queryClient.invalidateQueries({ queryKey: ["model-connections", namespace] });
      onChange({ modelConnectionRef: connection.metadata.name, provider: connection.spec.provider, model: connection.spec.models[0] });
      setAdding(false);
    } catch (err) { setError(err instanceof Error ? err.message : "Could not save connection"); } finally { setSaving(false); }
  }
  return <div className="space-y-4">
    <div className="space-y-2">
      <Label>Model connection</Label>
      <Select value={connectionRef || "legacy"} onValueChange={(value) => {
        if (value === "legacy") { onChange({ modelConnectionRef: undefined, provider: native ? "deepseek" : provider, model: native ? "deepseek-chat" : model }); return; }
        const connection = connections.data?.find((item) => item.metadata.name === value);
        if (connection) onChange({ modelConnectionRef: value, provider: connection.spec.provider, model: connection.spec.models[0] });
      }}>
        <SelectTrigger aria-label="Model connection"><SelectValue /></SelectTrigger>
        <SelectContent>
          <SelectItem value="legacy">{native ? "Existing DeepSeek host route" : "Configure provider directly"}</SelectItem>
          {(connections.data || []).filter((connection) => native ? !!connection.spec.credentialProfile : !connection.spec.credentialProfile).map((connection) => <SelectItem key={connection.metadata.name} value={connection.metadata.name} disabled={connection.spec.disabled}>{connection.metadata.name} · {connection.spec.provider}{connection.spec.disabled ? " (disabled)" : ""}</SelectItem>)}
        </SelectContent>
      </Select>
      {connections.isError && <p role="alert" className="text-sm text-destructive">Could not load model connections. {connections.error.message}</p>}
      {selected && <p className="break-all text-xs text-muted-foreground">{selected.spec.endpoint}</p>}
    </div>
    <div className="space-y-2">
      <Label htmlFor="native-model">Model</Label>
      {selected ? <Select value={model} onValueChange={(value) => onChange({ modelConnectionRef: connectionRef, provider: selected.spec.provider, model: value })}>
        <SelectTrigger id="native-model"><SelectValue placeholder="Choose model" /></SelectTrigger>
        <SelectContent>{selected.spec.models.map((id) => <SelectItem key={id} value={id}>{id}</SelectItem>)}</SelectContent>
      </Select> : <Input id="native-model" value={model} onChange={(event) => onChange({ modelConnectionRef: connectionRef, provider, model: event.target.value })} placeholder="deepseek-chat" />}
    </div>
    <p className="text-xs text-muted-foreground">{native ? "The host must approve this connection for the selected runtime and Agent. Credentials stay on the host. Saving a connection does not grant access." : "Saved connections work in YAML and the create wizard. Leave the Secret reference empty for an unauthenticated local server."}</p>
    <Button type="button" variant="outline" size="sm" onClick={() => setAdding(!adding)}>{adding ? "Cancel" : "Add connection"}</Button>
    {adding && <div className="space-y-3 rounded border p-3" data-testid="add-model-connection">
      {([ ["name", "Connection name", "team-models"], ["provider", "Provider", "anthropic, openai, or a custom provider"], ["endpoint", "API endpoint", "http://framework:8080/v1/chat/completions"], ...(native ? [["credentialProfile", "Host credential profile", "team-provider-key"] as const] : [["secretRef", "Kubernetes Secret (optional)", "provider-key"] as const]), ["models", "Model IDs (comma separated)", "model-id"] ] as const).map(([key, label, placeholder]) => <div key={key} className="space-y-1"><Label htmlFor={`connection-${key}`}>{label}</Label><Input id={`connection-${key}`} value={draft[key]} placeholder={placeholder} onChange={(event) => setDraft({ ...draft, [key]: event.target.value })} /></div>)}
      <Label>API protocol</Label>
      <Select value={draft.protocol} onValueChange={(value) => setDraft({ ...draft, protocol: value as typeof draft.protocol })}>
        <SelectTrigger aria-label="API protocol"><SelectValue /></SelectTrigger>
        <SelectContent><SelectItem value="openai-chat">OpenAI compatible chat completions</SelectItem>{native && <SelectItem value="anthropic-messages">Anthropic Messages</SelectItem>}</SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{native ? "Use the name of a credential mapping configured by your host operator." : "The maintained Hermes adapter uses OpenAI compatible endpoints."} For other API protocols, connect through an OpenAI compatible gateway.</p>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <Button type="button" size="sm" disabled={!draft.name || !draft.provider || !draft.endpoint || (native && !draft.credentialProfile) || !draft.models || saving} onClick={save}>Save connection</Button>
    </div>}
  </div>;
}
