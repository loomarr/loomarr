import { Activity, HeartPulse, RadioTower } from "lucide-react";
import type { ReactNode } from "react";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { ApplicationDiagnostics, type ApplicationFilters } from "../application-diagnostics";
import { StartupReportPage } from "../startup-report-page";

type DiagnosticsView = "health" | "application" | "playout";

type DiagnosticsSearch = ApplicationFilters & {
  view: DiagnosticsView;
  processId?: string;
};

const DiagnosticsPage = ({
  search,
  onSearchChange,
  playout,
  onOpenRelated,
}: {
  search: DiagnosticsSearch;
  onSearchChange: (search: DiagnosticsSearch) => void;
  playout?: ReactNode;
  onOpenRelated?: (kind: "channel" | "job", id: string) => void;
}) => {
  const tabs = [
    { id: "health" as const, label: "App Health", icon: HeartPulse },
    { id: "application" as const, label: "Application", icon: Activity },
    { id: "playout" as const, label: "Playout", icon: RadioTower },
  ];
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title="Diagnostics"
        description="Current health and bounded technical evidence for this Loomarr instance."
      />
      <nav className="flex gap-1 border-border border-b px-4 pt-2 sm:px-6" aria-label="Diagnostics views">
        {tabs.map(({ id, label, icon: Icon }) => (
          <button
            key={id}
            type="button"
            aria-current={search.view === id ? "page" : undefined}
            className={`inline-flex items-center gap-2 rounded-t-md border px-3 py-2 text-sm ${
              search.view === id
                ? "border-border border-b-background bg-background font-medium text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
            onClick={() => onSearchChange({ ...search, view: id })}
          >
            <Icon aria-hidden className="size-4" />
            {label}
          </button>
        ))}
      </nav>
      <div className="min-h-0 flex-1 overflow-y-auto p-4 sm:p-6">
        {search.view === "health" && <StartupReportPage embedded />}
        {search.view === "application" && (
          <ApplicationDiagnostics
            filters={search}
            onFiltersChange={(filters) => onSearchChange({ ...search, ...filters })}
            onOpenProcess={(processId) => onSearchChange({ ...search, view: "playout", processId })}
            onOpenRelated={onOpenRelated}
          />
        )}
        {search.view === "playout" && playout}
      </div>
    </div>
  );
};

export type { DiagnosticsSearch, DiagnosticsView };
export { DiagnosticsPage };
