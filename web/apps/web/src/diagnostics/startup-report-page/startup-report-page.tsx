import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import type { HealthCheck } from "@loomarr/api/models/healthCheck";
import type { HealthReport } from "@loomarr/api/models/healthReport";
import type { StartupCheck } from "@loomarr/api/models/startupCheck";
import type { StartupReport } from "@loomarr/api/models/startupReport";
import { useQueryClient } from "@tanstack/react-query";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { Button } from "@/components/ui/button";
import { StatusDot, type StatusTone } from "@/components/ui/status-dot";

const duration = (millis: number, pending = false) => {
  if (pending) return "In progress";
  if (millis < 1_000) return `${millis}ms`;
  return `${(millis / 1_000).toFixed(millis < 10_000 ? 1 : 0)}s`;
};

const dateTime = (millis?: number) => {
  if (!millis) return "Not checked yet";
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(
    new Date(millis),
  );
};

const statusTone = (status: string): StatusTone => {
  if (status === "passed" || status === "ready" || status === "healthy") return "ok";
  if (status === "warning" || status === "degraded") return "warn";
  if (status === "failed" || status === "blocked" || status === "unhealthy" || status === "stale")
    return "error";
  if (status === "pending" || status === "starting") return "pending";
  return "off";
};

const healthStateCopy = (state: HealthReport["state"]) => {
  if (state === "healthy") return "Healthy";
  if (state === "degraded") return "Needs attention";
  if (state === "unhealthy") return "Unhealthy";
  return "Checking";
};

const startupStateCopy = (report: StartupReport) => {
  if (report.state === "ready") return "Ready";
  if (report.state === "degraded") return "Started with warnings";
  if (report.state === "blocked") return "Startup blocked";
  return "Starting";
};

const readableStatus = (status: string) =>
  status === "skipped" ? "Not configured" : `${status.charAt(0).toUpperCase()}${status.slice(1)}`;

const CheckIdentity = ({ check }: { check: HealthCheck }) => (
  <>
    <span className="font-medium">{check.label}</span>
    <span className="text-muted-foreground text-xs">
      {check.required ? "Required" : "Optional"} · {check.mode === "continuous" ? "Monitored" : "Startup"}
    </span>
  </>
);

const HealthCheckRow = ({ check }: { check: HealthCheck }) => (
  <tr className="border-border border-t align-top">
    <th scope="row" className="px-4 py-3 text-left font-normal">
      <span className="flex flex-col gap-0.5">
        <CheckIdentity check={check} />
      </span>
    </th>
    <td className="px-4 py-3">
      <span className="inline-flex items-center gap-2">
        <StatusDot tone={statusTone(check.status)} label={readableStatus(check.status)} />
        {readableStatus(check.status)}
      </span>
    </td>
    <td className="px-4 py-3 text-muted-foreground text-xs">
      <time dateTime={check.observedAt ? new Date(check.observedAt).toISOString() : undefined}>
        {dateTime(check.observedAt)}
      </time>
    </td>
    <td className="px-4 py-3 text-muted-foreground">
      {check.detail || "—"}
      {check.remediationRoute && (
        <a className="ml-2 text-signal underline underline-offset-4" href={check.remediationRoute}>
          Open
        </a>
      )}
    </td>
  </tr>
);

const HealthCheckCard = ({ check }: { check: HealthCheck }) => (
  <li className="space-y-2 border-border border-t px-4 py-3 first:border-t-0">
    <div className="flex items-start justify-between gap-3">
      <span className="flex flex-col gap-0.5">
        <CheckIdentity check={check} />
      </span>
      <span className="inline-flex shrink-0 items-center gap-2 text-sm">
        <StatusDot tone={statusTone(check.status)} label={readableStatus(check.status)} />
        {readableStatus(check.status)}
      </span>
    </div>
    <p className="text-muted-foreground text-sm">
      {check.detail || "No additional detail."}
      {check.remediationRoute && (
        <a className="ml-2 text-signal underline underline-offset-4" href={check.remediationRoute}>
          Open
        </a>
      )}
    </p>
    <time
      className="block text-muted-foreground text-xs"
      dateTime={check.observedAt ? new Date(check.observedAt).toISOString() : undefined}
    >
      Last checked {dateTime(check.observedAt)}
    </time>
  </li>
);

const CurrentHealthCard = ({
  report,
  refreshing = false,
  onRefresh,
}: {
  report: HealthReport;
  refreshing?: boolean;
  onRefresh?: () => void;
}) => (
  <section
    className="overflow-hidden rounded-lg border border-border bg-card"
    aria-labelledby="current-health-title"
  >
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-4">
      <div className="min-w-0">
        <h2 id="current-health-title" className="font-medium text-lg">
          Current Health
        </h2>
        <p className="text-muted-foreground text-xs">
          Updated{" "}
          <time dateTime={new Date(report.updatedAt).toISOString()}>{dateTime(report.updatedAt)}</time>
          {report.nextRefreshAt && (
            <>
              {" · Next check expected "}
              <time dateTime={new Date(report.nextRefreshAt).toISOString()}>
                {dateTime(report.nextRefreshAt)}
              </time>
            </>
          )}
        </p>
      </div>
      <span className="inline-flex items-center gap-2 text-sm">
        <StatusDot tone={statusTone(report.state)} label={healthStateCopy(report.state)} />
        {healthStateCopy(report.state)}
      </span>
      <span className="font-mono text-muted-foreground text-xs">{report.version}</span>
      {onRefresh && (
        <Button className="ml-auto" variant="outline" size="sm" disabled={refreshing} onClick={onRefresh}>
          {refreshing ? "Checking…" : "Check again"}
        </Button>
      )}
    </div>
    <div className="hidden overflow-x-auto md:block">
      <table className="w-full min-w-[44rem] text-sm">
        <caption className="sr-only">Current Loomarr health checks</caption>
        <thead className="border-border border-t bg-muted/30 text-muted-foreground text-xs">
          <tr>
            {(["Check", "Status", "Last checked", "Detail"] as const).map((label) => (
              <th key={label} scope="col" className="px-4 py-2 text-left font-medium">
                {label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {report.checks.map((check) => (
            <HealthCheckRow key={check.key} check={check} />
          ))}
        </tbody>
      </table>
    </div>
    <ul className="border-border border-t md:hidden" aria-label="Current Loomarr health checks">
      {report.checks.map((check) => (
        <HealthCheckCard key={check.key} check={check} />
      ))}
    </ul>
  </section>
);

const StartupCheckRow = ({ check }: { check: StartupCheck }) => (
  <tr className="border-border border-t align-top">
    <th scope="row" className="px-4 py-3 text-left font-medium">
      {check.label}
    </th>
    <td className="px-4 py-3">
      <span className="inline-flex items-center gap-2">
        <StatusDot tone={statusTone(check.status)} label={readableStatus(check.status)} />
        {readableStatus(check.status)}
      </span>
    </td>
    <td className="px-4 py-3 font-mono text-muted-foreground text-xs">
      {duration(check.durationMillis, check.status === "pending")}
    </td>
    <td className="px-4 py-3 text-muted-foreground">{check.detail || "—"}</td>
  </tr>
);

const StartupReportCard = ({ report }: { report: StartupReport }) => (
  <section
    className="overflow-hidden rounded-lg border border-border bg-card"
    aria-labelledby={`report-${report.id}`}
  >
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 py-3">
      <h3 id={`report-${report.id}`} className="font-medium">
        <time dateTime={new Date(report.generationStartedAt).toISOString()}>
          {dateTime(report.generationStartedAt)}
        </time>
      </h3>
      <span className="inline-flex items-center gap-2 text-sm">
        <StatusDot tone={statusTone(report.state)} label={startupStateCopy(report)} />
        {startupStateCopy(report)}
      </span>
      <span className="font-mono text-muted-foreground text-xs">{report.version}</span>
      <span className="ml-auto text-muted-foreground text-xs">
        {duration(report.durationMillis, report.state === "starting")}
      </span>
    </div>
    <div className="overflow-x-auto">
      <table className="w-full min-w-[44rem] text-sm">
        <caption className="sr-only">Startup checks for application generation {report.generation}</caption>
        <thead className="border-border border-t bg-muted/30 text-muted-foreground text-xs">
          <tr>
            {(["Check", "Status", "Duration", "Detail"] as const).map((label) => (
              <th key={label} scope="col" className="px-4 py-2 text-left font-medium">
                {label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {report.checks.map((check) => (
            <StartupCheckRow key={check.key} check={check} />
          ))}
        </tbody>
      </table>
    </div>
  </section>
);

const StartupReportPage = ({ embedded = false }: { embedded?: boolean }) => {
  const queryClient = useQueryClient();
  const health = diagnosticsApi.useGetCurrentHealth({
    query: { retry: false, refetchInterval: 30_000 },
  });
  const reports = diagnosticsApi.useListStartupReports({ limit: 20 }, { query: { retry: false } });
  const refresh = diagnosticsApi.useRefreshCurrentHealth({
    mutation: {
      onSuccess: () =>
        queryClient.invalidateQueries({ queryKey: diagnosticsApi.getGetCurrentHealthQueryKey() }),
    },
  });

  if (health.isError)
    return (
      <div className="p-6">
        <ErrorState error={health.error} onRetry={() => health.refetch()} />
      </div>
    );
  const currentHealth = health.data?.status === 200 ? health.data.data : undefined;
  const reportBody = reports.data?.status === 200 ? reports.data.data : undefined;
  const previous = reportBody?.items.filter((report) => report.id !== reportBody.current.id) ?? [];

  const content = (
    <div
      className={embedded ? "space-y-6" : "min-h-0 flex-1 space-y-6 overflow-y-auto p-4 sm:p-6"}
      aria-live="polite"
    >
      {currentHealth ? (
        <CurrentHealthCard
          report={currentHealth}
          refreshing={refresh.isPending}
          onRefresh={() => refresh.mutate()}
        />
      ) : (
        <p className="text-muted-foreground text-sm">Checking app health…</p>
      )}

      <section className="space-y-3" aria-labelledby="previous-startups-title">
        <div>
          <h2 id="previous-startups-title" className="font-medium text-lg">
            Previous Startups
          </h2>
          <p className="text-muted-foreground text-sm">
            Retained boot reports from earlier application generations.
          </p>
        </div>
        {reports.isError ? (
          <p className="text-danger text-sm">Previous startup reports could not be loaded.</p>
        ) : !reportBody ? (
          <p className="text-muted-foreground text-sm">Loading startup history…</p>
        ) : previous.length === 0 ? (
          <p className="rounded-lg border border-border bg-card px-4 py-6 text-muted-foreground text-sm">
            No previous startups are retained yet.
          </p>
        ) : (
          previous.map((report) => <StartupReportCard key={report.id} report={report} />)
        )}
      </section>
    </div>
  );
  if (embedded) return content;
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title="App Health"
        description="What is healthy now, what needs attention, and how previous Loomarr startups completed."
      />
      {content}
    </div>
  );
};

export { CurrentHealthCard, StartupReportCard, StartupReportPage };
