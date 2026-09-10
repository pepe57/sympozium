import type { AgentRuntime } from "./api";

export function persistentHarnessName(runtime: AgentRuntime): "Pi" | "Hermes" | undefined {
  if (runtime.spec.contractVersion !== "v1alpha2" || runtime.spec.session?.protocol !== "openai-chat") return undefined;
  const adapter = runtime.spec.image?.match(/\/(pi|hermes)@sha256:/)?.[1];
  return adapter === "pi" ? "Pi" : adapter === "hermes" ? "Hermes" : undefined;
}

export function persistentHarnesses(runtimes: AgentRuntime[]): AgentRuntime[] {
  return runtimes.filter((runtime) => persistentHarnessName(runtime) !== undefined);
}

/**
 * Native Celln runtimes use the AgentRuntime CRD but execute on the Celln plane,
 * not as a Kubernetes harness. They must not be presented as harnesses.
 */
export function isNativeCellnRuntime(runtime: AgentRuntime): boolean {
  return !!runtime.spec.celln;
}

/** AgentRuntimes that represent Kubernetes/OCI harnesses (excludes native Celln runtimes). */
export function kubernetesHarnesses(runtimes: AgentRuntime[]): AgentRuntime[] {
  return runtimes.filter((runtime) => !isNativeCellnRuntime(runtime));
}
