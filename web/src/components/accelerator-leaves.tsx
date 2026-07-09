import { Badge } from "@/components/ui/badge";
import { Zap } from "lucide-react";
import { draDeviceDetail, groupAccelerators } from "@/lib/dra";
import type { DraDevice } from "@/lib/api";

/** Accelerators as tree leaves under a node — inventory from llmfit-dra
 * ResourceSlices, grouped per device flavour with a ×N count. Used by the
 * Placement & Density node cards and the topology's K8s node cards. */
export function AcceleratorLeaves({ devices }: { devices: DraDevice[] }) {
  const groups = groupAccelerators(devices);
  if (groups.length === 0) return null;
  return (
    <div className="pt-1 font-mono text-xs">
      <div className="flex items-center gap-1.5 text-muted-foreground">
        <Zap className="h-3 w-3" />
        <span>accelerators</span>
      </div>
      {groups.map((g, i) => (
        <div
          key={g.key}
          className={g.healthy ? "flex items-baseline gap-1.5" : "flex items-baseline gap-1.5 text-destructive"}
          title={g.healthy ? g.names.join(", ") : `${g.names.join(", ")} — ${g.reasons.join(", ") || "unhealthy"}`}
        >
          <span className="text-muted-foreground/60 select-none">
            {i === groups.length - 1 ? "└─" : "├─"}
          </span>
          <span className="uppercase text-[10px] tracking-wider text-muted-foreground shrink-0">
            {g.count > 1 ? `${g.count}× ${g.kind}` : g.kind}
          </span>
          <span className="truncate">{draDeviceDetail(g.sample)}</span>
          {!g.healthy && (
            <Badge variant="destructive" className="text-[9px] px-1 py-0 shrink-0">
              {g.reasons[0] || "unhealthy"}
            </Badge>
          )}
        </div>
      ))}
    </div>
  );
}
