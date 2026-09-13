import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerDecisionActivityWireDTOKind } from "@loomarr/api/models/fillerDecisionActivityWireDTOKind";
import type { FillerDecisionDiagnosticDTO } from "@loomarr/api/models/fillerDecisionDiagnosticDTO";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { formatRelative, formatUntil, pluralize } from "@loomarr/core/format";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Badge, type BadgeProps } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Disclosure } from "@/components/ui/disclosure";

const ACTIVITY_LABELS: Record<FillerDecisionActivityWireDTOKind, string> = {
  automatic_admit: "Added automatically",
  automatic_reject: "Skipped automatically",
  review_requested: "Needs a decision",
  review_admit: "Added after review",
  review_reject: "Skipped after review",
  correction: "Corrected",
  review_abandoned: "Skipped for later",
  restore: "Restored",
  reversal: "Reversed",
};

type ActivityPresentation = { label: string; variant: BadgeProps["variant"] };

const activityPresentation = (
  kind: FillerDecisionActivityWireDTOKind,
  applicationMode: unknown,
): ActivityPresentation => {
  if (kind === "automatic_admit" || kind === "automatic_reject") {
    if (applicationMode === "shadow") {
      return {
        label: kind === "automatic_admit" ? "Would add (preview)" : "Would skip (preview)",
        variant: "caution",
      };
    }
    if (applicationMode !== "applied") {
      return { label: "Status unavailable", variant: "caution" };
    }
  }
  return {
    label: ACTIVITY_LABELS[kind],
    variant:
      kind === "automatic_admit" || kind === "review_admit"
        ? "signal"
        : kind === "automatic_reject" || kind === "review_reject"
          ? "neutral"
          : "caution",
  };
};

const recoveryCopy = (row: FillerDecisionDiagnosticDTO) => {
  switch (row.recovery.action) {
    case "configure_provider":
      return {
        title: "Connect your AI service",
        detail: "Loomarr couldn’t reach the AI service. Check the connection settings, then try again.",
        action: "Check AI connection",
      };
    case "adjust_budget":
      return {
        title: "Processing is paused",
        detail: "Loomarr reached the processing limit. You can raise the limit or wait for the next run.",
        action: "Review limits",
      };
    case "retry_extraction":
      return {
        title: "This clip couldn’t be processed",
        detail: "The work that already finished won’t be lost.",
        action: "Try again",
      };
    case "inspect_media":
      return {
        title: "Check this clip",
        detail: "Loomarr couldn’t read this file reliably. Open it and make sure it plays correctly.",
        action: "Open clip",
      };
    default:
      return {
        title: "Check filler settings",
        detail: "A filler setting or category isn’t valid. Fix it before Loomarr checks this clip again.",
        action: "Open filler settings",
      };
  }
};

const DiagnosticRecovery = ({ row }: { row: FillerDecisionDiagnosticDTO }) => {
  const queryClient = useQueryClient();
  const [retryID] = useState(() => crypto.randomUUID());
  const copy = recoveryCopy(row);
  const retryWhen = row.recovery.retryAt ? formatUntil(row.recovery.retryAt) : undefined;
  const automaticRetryText =
    retryWhen === "expired"
      ? "Loomarr is trying again now."
      : `Loomarr will try again ${retryWhen ?? "soon"}.`;
  const retry = fillerApi.useActOnFillerDiagnostic({
    mutation: {
      onSuccess: async () => {
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: fillerApi.getFillerDecisionOverviewQueryKey() }),
          queryClient.invalidateQueries({ queryKey: fillerApi.getFillerReadinessQueryKey() }),
        ]);
      },
      onSettled: async () => {
        await queryClient.invalidateQueries({ queryKey: fillerApi.getFillerDecisionDiagnosticsQueryKey() });
      },
    },
  });
  const problem = retry.error ? toProblem(retry.error) : undefined;

  return (
    <div className="min-w-0 flex-1">
      <div className="flex flex-wrap items-center gap-2">
        <p id={`diagnostic-${row.id}`} className="font-medium text-sm">
          {copy.title}
        </p>
        <Badge variant={row.recovery.mode === "automatic_retry" ? "signal" : "caution"}>
          {row.recovery.mode === "automatic_retry"
            ? "Trying again"
            : row.recovery.mode === "manual_retry"
              ? "Ready to retry"
              : row.recovery.mode === "inspection"
                ? "Check required"
                : "Settings needed"}
        </Badge>
      </div>
      <p className="mt-1 text-muted-foreground text-sm">{copy.detail}</p>
      <p className="mt-1 text-muted-foreground text-xs">Clip {row.clipHash.slice(0, 10)}…</p>
      <details className="mt-2 text-muted-foreground text-xs">
        <summary className="w-fit cursor-pointer">Technical details</summary>
        <p className="mt-1 break-all font-mono">
          Issue code: {row.code} · Clip ID: {row.clipHash}
        </p>
      </details>
      {row.recovery.mode === "automatic_retry" ? (
        <p className="mt-3 text-signal text-sm">{automaticRetryText} Nothing you need to do.</p>
      ) : null}
      {row.recovery.mode === "manual_retry" ? (
        <Button
          className="mt-3"
          size="sm"
          disabled={retry.isPending}
          onClick={() => retry.mutate({ id: row.id, data: { actionId: retryID, action: "retry" } })}
        >
          {retry.isPending ? "Starting retry…" : copy.action}
        </Button>
      ) : null}
      {(row.recovery.mode === "configuration" || row.recovery.mode === "inspection") &&
      row.recovery.destination ? (
        <Button
          className="mt-3"
          size="sm"
          variant="outline"
          render={
            <a
              href={row.recovery.destination}
              {...(row.recovery.mode === "inspection" ? { target: "_blank", rel: "noreferrer" } : {})}
            />
          }
        >
          {copy.action}
        </Button>
      ) : null}
      {problem ? (
        <p role="alert" className="mt-3 text-danger text-sm">
          The retry didn&apos;t start. This clip is still on hold. {problem.detail ?? problem.title}
        </p>
      ) : null}
    </div>
  );
};

const FillerManage = () => {
  const { isAdmin } = useAuth();
  const [diagnosticsOpen, setDiagnosticsOpen] = useState(false);
  const activityQuery = fillerApi.useFillerDecisionActivity({ limit: 100 });
  const diagnosticsQuery = fillerApi.useFillerDecisionDiagnostics(
    { limit: 100 },
    { query: { enabled: isAdmin && diagnosticsOpen } },
  );
  const activity = unwrap(activityQuery.data, (body) => body);
  const diagnostics = unwrap(diagnosticsQuery.data, (body) => body);

  return (
    <div className="flex flex-col gap-6">
      <section aria-labelledby="manage-tools-heading">
        <div className="mb-3">
          <h2 id="manage-tools-heading" className="font-semibold text-lg">
            Manage filler
          </h2>
          <p className="text-muted-foreground text-sm">
            Filler runs automatically. Use these tools only when you want to change something or fix a
            problem.
          </p>
        </div>
        <div className="grid gap-3 md:grid-cols-2">
          <Card className="p-4">
            <h3 className="font-medium">Processing</h3>
            <p className="mt-1 text-muted-foreground text-sm">
              Change how often Loomarr looks for clips and how much work it does at once.
            </p>
            {isAdmin ? (
              <Button className="mt-4" size="sm" variant="outline" render={<Link to="/filler/settings" />}>
                Processing settings
              </Button>
            ) : null}
          </Card>
          <Card className="p-4">
            <h3 className="font-medium">Categories</h3>
            <p className="mt-1 text-muted-foreground text-sm">
              Review the labels Loomarr uses to understand and organize filler.
            </p>
            <Button className="mt-4" size="sm" variant="outline" render={<Link to="/filler/taxonomy" />}>
              View categories
            </Button>
          </Card>
        </div>
      </section>

      <section aria-labelledby="activity-heading">
        <div className="mb-3">
          <h2 id="activity-heading" className="font-semibold text-lg">
            Recent activity
          </h2>
          <p className="text-muted-foreground text-sm">See what Loomarr added, skipped, or changed.</p>
        </div>
        {activityQuery.error ? (
          <ErrorState error={activityQuery.error} onRetry={() => activityQuery.refetch()} />
        ) : null}
        {!activity && !activityQuery.error ? (
          <Card aria-live="polite" className="p-4">
            Loading activity…
          </Card>
        ) : null}
        {activity?.rows.length === 0 ? (
          <Card className="p-4 text-muted-foreground text-sm">Nothing has happened yet.</Card>
        ) : null}
        {activity?.rows.length ? (
          <div className="overflow-hidden rounded-lg border border-border">
            {activity.rows.map((row) => {
              const presentation = activityPresentation(row.kind, row.applicationMode);
              return (
                <div
                  key={row.id}
                  className="flex flex-wrap items-center gap-3 border-border border-b p-3 last:border-b-0"
                >
                  <Badge variant={presentation.variant}>{presentation.label}</Badge>
                  <span className="min-w-0 flex-1 truncate font-mono text-muted-foreground text-xs">
                    Clip {row.clipHash.slice(0, 12)}…
                  </span>
                  <span className="text-muted-foreground text-xs">{formatRelative(row.createdAt)}</span>
                </div>
              );
            })}
          </div>
        ) : null}
      </section>

      {isAdmin ? (
        <Disclosure open={diagnosticsOpen} onOpenChange={setDiagnosticsOpen}>
          <Card className="overflow-hidden" id="diagnostics">
            <div className="flex flex-wrap items-center gap-3 p-4">
              <div className="min-w-0 flex-1">
                <h2 className="font-semibold text-lg">Needs attention</h2>
                <p className="text-muted-foreground text-sm">
                  See what stopped and how to get it moving again.
                </p>
              </div>
              {diagnostics ? (
                <Badge variant={diagnostics.total > 0 ? "caution" : "neutral"}>
                  {pluralize(diagnostics.total, "current issue")}
                </Badge>
              ) : null}
              <Disclosure.Trigger label={`${diagnosticsOpen ? "Hide" : "Show"} issues`} />
            </div>
            <Disclosure.Panel className="space-y-5 border-border border-t p-4">
              {diagnosticsOpen ? (
                <>
                  {diagnosticsQuery.error ? (
                    <ErrorState error={diagnosticsQuery.error} onRetry={() => diagnosticsQuery.refetch()} />
                  ) : null}
                  {diagnostics?.rows.length === 0 ? (
                    <p className="text-muted-foreground text-sm">
                      Everything is working. Nothing needs your attention.
                    </p>
                  ) : null}
                  {diagnostics?.rows.map((row) => (
                    <article
                      key={row.id}
                      aria-labelledby={`diagnostic-${row.id}`}
                      className="flex flex-col gap-3 rounded-md border border-border p-4 sm:flex-row sm:items-start"
                    >
                      <DiagnosticRecovery row={row} />
                      <span className="shrink-0 text-muted-foreground text-xs">
                        {formatRelative(row.createdAt)}
                      </span>
                    </article>
                  ))}
                </>
              ) : null}
            </Disclosure.Panel>
          </Card>
        </Disclosure>
      ) : null}
    </div>
  );
};

export { FillerManage };
