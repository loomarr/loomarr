import { Activity, HeartPulse, RadioTower } from "lucide-react";
import type { ReactNode } from "react";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { ApplicationDiagnostics, type ApplicationFilters } from "../application-diagnostics";
import { StartupReportPage } from "../startup-report-page";
import { SupportBundle } from "../support-bundle";

type DiagnosticsView = "logs" | "health" | "process";

type DiagnosticsSearch = ApplicationFilters & {
  view: DiagnosticsView;
  processId?: string;
  processRange: "1h" | "6h" | "24h";
  processStatus: "all" | "running" | "succeeded" | "failed" | "cancelled";
  processPurpose: string;
  processChannelId: string;
  processJobId: string;
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
    { id: "logs" as const, label: "Logs", icon: Activity },
    { id: "health" as const, label: "Current Health", icon: HeartPulse },
    { id: "process" as const, label: "Media processes", icon: RadioTower },
  ];
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title="Diagnostics"
        description="Find what happened and check whether this Loomarr server is healthy."
        actions={
          <SupportBundle
            initialRange={search.range}
            correlations={{
              requestId: search.requestId,
              playbackSessionId: search.playbackSessionId,
              channelId: (search.channelId ?? search.processChannelId) || undefined,
              scheduleBlockId: search.scheduleBlockId,
              jobId: (search.jobId ?? search.processJobId) || undefined,
              processRunId: search.processRunId ?? search.processId,
            }}
          />
        }
      />
      <nav
        className="flex gap-1 overflow-x-auto border-border border-b px-4 pt-2 pb-2 sm:px-6"
        aria-label="Diagnostics views"
      >
        {tabs.map(({ id, label, icon: Icon }) => (
          <button
            key={id}
            type="button"
            aria-current={search.view === id ? "page" : undefined}
            className={`inline-flex shrink-0 cursor-pointer items-center gap-2 whitespace-nowrap rounded-md px-3 py-1.5 text-sm transition-colors ${
              search.view === id
                ? "bg-signal-tint-15 font-medium text-signal"
                : "text-muted-foreground hover:bg-accent hover:text-foreground"
            }`}
            onClick={() =>
              onSearchChange({ ...search, view: id, ...(id === "process" ? {} : { processId: undefined }) })
            }
          >
            <Icon aria-hidden className="hidden size-4 sm:block" />
            {label}
          </button>
        ))}
      </nav>
      <div className="min-h-0 flex-1 overflow-y-auto p-4 sm:p-6">
        {search.view === "logs" && (
          <ApplicationDiagnostics
            filters={search}
            onFiltersChange={(filters) => onSearchChange({ ...search, ...filters })}
            onOpenProcess={(processId) => onSearchChange({ ...search, view: "process", processId })}
            onOpenRelated={onOpenRelated}
          />
        )}
        {search.view === "health" && <StartupReportPage embedded />}
        {search.view === "process" && playout}
      </div>
    </div>
  );
};

export type { DiagnosticsSearch, DiagnosticsView };
export { DiagnosticsPage };
