import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import type { HealthCheck } from "@loomarr/api/models/healthCheck";
import { HealthCheckStatus } from "@loomarr/api/models/healthCheckStatus";
import type { HealthReport } from "@loomarr/api/models/healthReport";
import { HealthReportState } from "@loomarr/api/models/healthReportState";
import {
  Stack,
  StatusWord,
  type StatusWordKind,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
  Text,
} from "@loomarr/design-system";
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
    <Text as="span" textRole="cardLabel">
      {check.label}
    </Text>
    <Text as="span" textRole="hint" tone="secondary">
      {check.required ? "Required" : "Optional"} · {check.mode === "continuous" ? "Monitored" : "Startup"}
    </Text>
  </>
);

const HealthCheckRow = ({ check }: { check: HealthCheck }) => (
  <TableRow>
    <TableCell rowHeader verticalAlign="top">
      <Stack gap="0.5" render="span">
        <CheckIdentity check={check} />
      </Stack>
    </TableCell>
    <TableCell verticalAlign="top">
      <Verdict word={checkWord(check.status)} />
    </TableCell>
    <TableCell verticalAlign="top">
      <time dateTime={check.observedAt ? new Date(check.observedAt).toISOString() : undefined}>
        <Text as="span" textRole="hint" tone="secondary">
          {dateTime(check.observedAt)}
        </Text>
      </time>
    </TableCell>
    <TableCell verticalAlign="top">
      <span className="text-muted-foreground">
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
      </span>
    </TableCell>
  </TableRow>
);

const CurrentHealthCard = ({ report }: { report: HealthReport }) => {
  return (
    <section
      className="border-border bg-card overflow-hidden rounded-lg border"
      aria-label="Current health summary"
    >
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-4">
        <div className="min-w-0 flex-1">
          <Text as="p" textRole="headline">
            <Verdict word={stateWord(report.state)} />
          </Text>
          <Text as="p" textRole="hint" tone="secondary">
            Continuously monitored
          </Text>
        </div>
        <Text as="span" textRole="mono">
          {report.version}
        </Text>
      </div>
      <div className="border-border border-t">
        <Table caption="Current health checks" framed={false} minWidth={704}>
          <TableHeader>
            <TableRow>
              {(["Check", "Status", "Last checked", "Detail"] as const).map((label) => (
                <TableHeaderCell key={label}>{label}</TableHeaderCell>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {report.checks.map((check) => (
              <HealthCheckRow key={check.key} check={check} />
            ))}
          </TableBody>
        </Table>
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
        <Text as="p" textRole="compact" tone="secondary">
          Checking app health…
        </Text>
      )}
    </div>
  );
  if (embedded) return content;
  return (
    <Stack className="h-full min-h-0">
      <PageHeader title="Current Health" description="What is working now and what needs your attention." />
      {content}
    </Stack>
  );
};

export { CurrentHealthCard, StartupReportPage };
