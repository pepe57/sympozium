import { useCallback, useSyncExternalStore } from "react";
import type { AgentRun } from "@/lib/api";

const STORAGE_KEY = "sympozium_runs_last_seen";

// Seed the watermark on first load so that any runs created after this
// moment are treated as "new". Without this, the badge never appears
// until the user manually visits /runs.
if (!localStorage.getItem(STORAGE_KEY)) {
  localStorage.setItem(STORAGE_KEY, new Date().toISOString());
}

// Notify all subscribers when the watermark changes (cross-component sync).
const listeners = new Set<() => void>();
function emit() {
  listeners.forEach((fn) => fn());
}

function getSnapshot(): string {
  return localStorage.getItem(STORAGE_KEY) || "";
}

function subscribe(cb: () => void) {
  listeners.add(cb);
  // Also listen for cross-tab changes.
  const onStorage = (e: StorageEvent) => {
    if (e.key === STORAGE_KEY) cb();
  };
  window.addEventListener("storage", onStorage);
  return () => {
    listeners.delete(cb);
    window.removeEventListener("storage", onStorage);
  };
}

/**
 * Hook that tracks which runs the user has "seen".
 *
 * - `isUnseen(run)` — true if the run was created after the last-seen watermark
 * - `unseenCount(runs)` — number of unseen runs in a list
 * - `markAllSeen()` — advance the watermark to now (call on /runs mount)
 * - `markSeenUpTo(ts)` — advance watermark to a specific timestamp
 */
export function useRunsSeen() {
  const raw = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  const watermark = raw ? new Date(raw).getTime() : 0;

  const isUnseen = useCallback(
    (run: AgentRun) => {
      if (!watermark) return false;
      const created = new Date(run.metadata.creationTimestamp || "").getTime();
      return created > watermark;
    },
    [watermark],
  );

  const unseenCount = useCallback(
    (runs: AgentRun[]) => runs.filter(isUnseen).length,
    [isUnseen],
  );

  const markAllSeen = useCallback(() => {
    localStorage.setItem(STORAGE_KEY, new Date().toISOString());
    emit();
  }, []);

  const markSeenUpTo = useCallback((ts: string) => {
    const proposed = new Date(ts).getTime();
    if (proposed > watermark) {
      localStorage.setItem(STORAGE_KEY, new Date(proposed).toISOString());
      emit();
    }
  }, [watermark]);

  return { isUnseen, unseenCount, markAllSeen, markSeenUpTo, watermark };
}
