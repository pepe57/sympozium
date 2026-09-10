import type { AgentExecutionDefaults, CellnSelection } from "./api";

export interface WizardExecution {
  executionBackend?: "job" | "celln";
  executionLifecycle?: "one-shot" | "enduring";
  borrowedTools?: CellnSelection["toolRefs"];
  runtimeRef?: string;
  model: string;
  provider?: string;
  modelConnectionRef?: string;
}

// Shared by API creation and YAML preview: neither may silently lose tool refs.
export function executionFromWizard(form: WizardExecution): AgentExecutionDefaults {
  if (form.executionBackend !== "celln") return { backend: "job", executionLifecycle: "one-shot", ...(form.modelConnectionRef ? { modelConnectionRef: form.modelConnectionRef } : {}) };
  return {
    backend: "celln",
    executionLifecycle: form.executionLifecycle || "one-shot",
    provider: form.modelConnectionRef ? undefined : form.provider || "deepseek",
    modelConnectionRef: form.modelConnectionRef,
    model: form.model,
    cellnSelection: {
      runtimeRef: form.runtimeRef || undefined,
      toolRefs: (form.borrowedTools || []).map((tool) => ({ ...tool })),
    },
    ...(form.executionLifecycle === "enduring" ? {
      enduring: { leaseSeconds: 600, maxTurns: 8, maxModelRequests: 24, maxOutputTokens: 8192 },
    } : {}),
  };
}

export function agentCreationSteps(celln: boolean) {
  return ["name", "plane", "runtime", ...(celln ? ["tools", "model"] : ["skills", "provider", "apikey", "model", "heartbeat", "channels"]), "confirm", "channelAction"] as const;
}
