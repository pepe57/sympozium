/**
 * EnsembleBuilderPage — page wrapper for the canvas-first ensemble builder.
 */

import { Breadcrumbs } from "@/components/breadcrumbs";
import { EnsembleBuilder } from "@/components/ensemble-builder";

export function EnsembleBuilderPage() {
  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <Breadcrumbs
          items={[
            { label: "Ensembles", to: "/ensembles" },
            { label: "New Ensemble" },
          ]}
        />
        <h1 className="text-2xl font-bold font-mono">New Ensemble</h1>
        <p className="text-sm text-muted-foreground">
          Add personas, draw relationships, and configure your agent team.
        </p>
      </div>

      <EnsembleBuilder />
    </div>
  );
}
