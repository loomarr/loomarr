import { Icon, icons, Stack } from "@loomarr/design-system";
import { NavTabs } from "@loomarr/ui";
import type { ReactNode } from "react";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { ApplicationDiagnostics } from "../application-diagnostics";
import type { ApplicationFilters } from "../application-filters";
import { StartupReportPage } from "../startup-report-page";
import { SupportBundle } from "../support-bundle";

type DiagnosticsView = "logs" | "health" | "process";

type DiagnosticsSearch = ApplicationFilters & {
  view: DiagnosticsView;
  processId?: string;
  processRange: "1h" | "6h" | "24h";
  processStatus: "all" | "running" | "succeeded" | "failed" | "cancelled" | "interrupted";
  processPurpose: string;
  processChannelId: string;
  processJobId: string;
};

const DiagnosticsPage = ({
  search,
  onSearchChange,
  playout,
  health,
  onOpenRelated,
}: {
  search: DiagnosticsSearch;
  onSearchChange: (search: DiagnosticsSearch) => void;
  playout?: ReactNode;
  // Live server state over the startup report: connected services, activity, restart (Q-H7).
  health?: ReactNode;
  onOpenRelated?: (kind: "channel" | "job", id: string) => void;
}) => {
  const tabs = [
    {
      id: "logs" as const,
      label: "Logs",
      icon: <Icon decorative glyph={icons.activity} size="compact" tone="inherit" />,
    },
    {
      id: "health" as const,
      label: "Current Health",
      icon: <Icon decorative glyph={icons.health} size="compact" tone="inherit" />,
    },
    {
      id: "process" as const,
      label: "Media processes",
      icon: <Icon decorative glyph={icons.process} size="compact" tone="inherit" />,
    },
  ];
  return (
    <Stack height="100%" minHeight={0}>
      <PageHeader
        title="Diagnostics"
        description="Find what happened and check whether this Loomarr server is healthy."
        actions={
          <SupportBundle
            initialRange={search.range === "all" ? "24h" : search.range}
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
      <Stack className="px-4 sm:px-6" paddingTop={8}>
        <NavTabs
          activeId={search.view}
          label="Diagnostics views"
          onSelect={(id) =>
            onSearchChange({
              ...search,
              view: id as DiagnosticsView,
              ...(id === "process" ? {} : { processId: undefined }),
            })
          }
          tabs={tabs}
        />
      </Stack>
      {/* ⚠ `position: relative` is load-bearing: visually-hidden (`sr-only`) text is absolutely
          positioned, and without a positioned scroller it stretches the document instead of
          scrolling in here (#3404, #2724). */}
      <Stack
        className="p-4 sm:p-6"
        data-testid="diagnostics-scroller"
        flex={1}
        flexShrink={1}
        minHeight={0}
        overflowY="auto"
        style={{ position: "relative" }}
      >
        {search.view === "logs" && (
          <ApplicationDiagnostics
            filters={search}
            onFiltersChange={(filters) => onSearchChange({ ...search, ...filters })}
            onOpenProcess={(processId) => onSearchChange({ ...search, view: "process", processId })}
            onOpenRelated={onOpenRelated}
          />
        )}
        {search.view === "health" && (
          <>
            {health}
            <StartupReportPage embedded />
          </>
        )}
        {search.view === "process" && playout}
      </Stack>
    </Stack>
  );
};

export type { DiagnosticsSearch, DiagnosticsView };
export { DiagnosticsPage };
