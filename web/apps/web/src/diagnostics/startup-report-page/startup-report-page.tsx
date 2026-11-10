import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import type { HealthCheck } from "@loomarr/api/models/healthCheck";
import { HealthCheckStatus } from "@loomarr/api/models/healthCheckStatus";
import type { HealthReport } from "@loomarr/api/models/healthReport";
import { HealthReportState } from "@loomarr/api/models/healthReportState";
import { StatusWord, type StatusWordKind } from "@loomarr/design-system";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PageHeader } from "@/components/loomarr/shell/page-header";

const dateTime = (millis?: number) => {
  if (!millis) return "Not checked yet";
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(
    new Date(millis),
  );
};

// Health speaks the same five words as the rest of Settings. A check still in flight has no
// verdict yet, so it keeps a plain "Checking" instead of borrowing one.
const checkWord = (status: HealthCheck["status"]): StatusWordKind | undefined => {
  if (status === HealthCheckStatus.passed) return "ok";
  if (status === HealthCheckStatus.skipped) return "notSetUp";
  if (status === HealthCheckStatus.pending) return undefined;
  return "attention";
};

const stateWord = (state: HealthReport["state"]): StatusWordKind | undefined => {
  if (state === HealthReportState.healthy) return "ok";
  if (state === HealthReportState.starting) return undefined;
  return "attention";
};

const Verdict = ({ word }: { word: StatusWordKind | undefined }) =>
  word ? <StatusWord kind={word} /> : <span className="text-muted-foreground">Checking</span>;

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
      <Verdict word={checkWord(check.status)} />
    </td>
    <td className="text-muted-foreground px-4 py-3 text-xs">
      <time dateTime={check.observedAt ? new Date(check.observedAt).toISOString() : undefined}>
        {dateTime(check.observedAt)}
      </time>
    </td>
    <td className="text-muted-foreground px-4 py-3">
      {check.detail || "—"}
      {check.remediationRoute && (
        <>
          {/* A real separator, so the detail and its link never run together as text (#2734). */}
          <span aria-hidden> · </span>
          <a className="text-signal underline underline-offset-4" href={check.remediationRoute}>
            Open
          </a>
        </>
      )}
    </td>
  </tr>
);

const CurrentHealthCard = ({ report }: { report: HealthReport }) => {
  return (
    <section
      className="border-border bg-card overflow-hidden rounded-lg border"
      aria-label="Current health summary"
    >
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-4">
        <div className="min-w-0 flex-1">
          <p className="text-lg font-medium">
            <Verdict word={stateWord(report.state)} />
          </p>
          <p className="text-muted-foreground text-xs">Continuously monitored</p>
        </div>
        <span className="text-muted-foreground font-mono text-xs">{report.version}</span>
      </div>
      <div className="border-border border-t">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[44rem] text-sm">
            <caption className="sr-only">Current health checks</caption>
            <thead className="border-border bg-muted/30 text-muted-foreground border-t text-xs">
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
