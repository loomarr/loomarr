import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import type { HealthCheck } from "@loomarr/api/models/healthCheck";
import type { HealthReport } from "@loomarr/api/models/healthReport";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { StatusDot, type StatusTone } from "@/components/ui/status-dot";

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

const CurrentHealthCard = ({ report }: { report: HealthReport }) => {
  return (
    <section
      className="overflow-hidden rounded-lg border border-border bg-card"
      aria-label="Current health summary"
    >
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-4">
        <div className="min-w-0 flex-1">
          <p className="inline-flex items-center gap-2 font-medium text-lg">
            <StatusDot tone={statusTone(report.state)} label={healthStateCopy(report.state)} />
            {healthStateCopy(report.state)}
          </p>
          <p className="text-muted-foreground text-xs">Continuously monitored</p>
        </div>
        <span className="font-mono text-muted-foreground text-xs">{report.version}</span>
      </div>
      <div className="border-border border-t">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[44rem] text-sm">
            <caption className="sr-only">Current health checks</caption>
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
      </div>
    </section>
  );
};

const StartupReportPage = ({ embedded = false }: { embedded?: boolean }) => {
  const health = diagnosticsApi.useGetCurrentHealth({
    query: { retry: false, refetchInterval: 10_000 },
  });

  if (health.isError)
    return (
      <div className="p-6">
        <ErrorState error={health.error} onRetry={() => health.refetch()} />
      </div>
    );
  const currentHealth = health.data?.status === 200 ? health.data.data : undefined;

  const content = (
    <div
      className={embedded ? "space-y-6" : "min-h-0 flex-1 space-y-6 overflow-y-auto p-4 sm:p-6"}
      aria-live="polite"
    >
      {currentHealth ? (
        <CurrentHealthCard report={currentHealth} />
      ) : (
        <p className="text-muted-foreground text-sm">Checking app health…</p>
      )}
    </div>
  );
  if (embedded) return content;
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader title="Current Health" description="What is working now and what needs your attention." />
      {content}
    </div>
  );
};

export { CurrentHealthCard, StartupReportPage };
