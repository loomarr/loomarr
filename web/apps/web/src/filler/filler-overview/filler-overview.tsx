import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerAcquisitionRunDTO } from "@loomarr/api/models/fillerAcquisitionRunDTO";
import type { FillerReadinessDTO } from "@loomarr/api/models/fillerReadinessDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatDuration, formatRelative, pluralize } from "@loomarr/core/format";
import { Link } from "@tanstack/react-router";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";

type Action = {
  title: string;
  description: string;
  label: string;
  to:
    | "/filler/attention"
    | "/filler/library"
    | "/filler/settings"
    | "/filler/sources"
    | "/channels/$id/filler";
};

const nextAction = (readiness: FillerReadinessDTO): Action | undefined => {
  switch (readiness.nextAction) {
    case "none":
      return undefined;
    case "enable_fetch":
      return {
        title: "Turn on a source",
        description: "No source is fetching filler automatically. Enable one to keep the library supplied.",
        label: "Review sources",
        to: "/filler/sources",
      };
    case "free_catalog_capacity":
      return {
        title: "Automatic fetching reached the library limit",
        description: "Curate the library or raise its ceiling to let unattended fetching continue.",
        label: "Review limits",
        to: "/filler/settings",
      };
    case "free_disk_capacity":
      return {
        title: "Automatic fetching reached the storage limit",
        description: "Free filler storage or raise its ceiling to let unattended fetching continue.",
        label: "Review limits",
        to: "/filler/settings",
      };
    case "retry_acquisition":
      return {
        title: "A filler acquisition failed",
        description: "Review the failed run and retry it from the source that produced it.",
        label: "Inspect the failure",
        to: "/filler/attention",
      };
    case "retry_failed_work":
      return {
        title: "Some prepared filler can be recovered",
        description: `${pluralize(readiness.actionCount ?? 0, "item")} can be retried or restored without downloading it again.`,
        label: "Recover filler",
        to: "/filler/attention",
      };
    case "review_incoming":
      return {
        title: "Filler needs a decision",
        description: `${pluralize(readiness.actionCount ?? 0, "item")} could not be filed safely without your judgment.`,
        label: "Review decisions",
        to: "/filler/attention",
      };
    case "add_filler":
      return {
        title: "Add playable filler",
        description:
          "The library does not have eligible commercials yet. Add or fetch filler to start filling breaks.",
        label: "Find filler",
        to: "/filler/sources",
      };
    case "improve_channel_coverage":
      return {
        title: "Improve a channel's filler coverage",
        description: "This channel is falling back to a loose match or the built-in bumper card.",
        label: "Review channel coverage",
        to: "/channels/$id/filler",
      };
  }
};

const acquisitionSummary = (run: FillerAcquisitionRunDTO) => {
  if (run.status === "queued") return "Waiting to start";
  if (run.status === "running") return `${pluralize(run.fetched, "download")} so far`;
  if (run.status === "error") return run.error || "The acquisition did not complete";
  if (run.empty > 0) return "The source returned no usable results";
  return `${pluralize(run.fetched, "download")} · ${pluralize(run.outcome.admitted, "admitted clip")}`;
};

const statusTone = (status: FillerAcquisitionRunDTO["status"]) => {
  if (status === "error") return "lock" as const;
  if (status === "running" || status === "queued") return "caution" as const;
  return "signal" as const;
};

const FillerOverview = () => {
  const query = fillerApi.useFillerReadiness();
  const readiness = unwrap(query.data, (body) => body);

  if (query.error) return <ErrorState error={query.error} onRetry={() => query.refetch()} />;
  if (!readiness) {
    return (
      <Card aria-live="polite" className="p-6">
        <p className="font-medium">Checking filler readiness…</p>
        <p className="mt-1 text-muted-foreground text-sm">
          Looking at fetching, preparation, decisions, and channel coverage.
        </p>
      </Card>
    );
  }

  const action = nextAction(readiness);
  const machineWork =
    readiness.pipeline.runnable + readiness.pipeline.scheduled + readiness.pipeline.inProgress;

  return (
    <div className="flex flex-col gap-6">
      <Card className={readiness.ready ? "border-signal/35 p-5" : "border-caution/40 p-5"}>
        <div className="flex flex-col items-start gap-4 sm:flex-row">
          <div className="min-w-0 flex-1">
            <Badge variant={readiness.ready ? "signal" : "caution"}>
              {readiness.ready ? "Ready" : "Action recommended"}
            </Badge>
            <h2 className="mt-3 font-semibold text-xl">{action?.title ?? "Filler is working on its own"}</h2>
            <p className="mt-1 max-w-3xl text-muted-foreground text-sm">
              {action?.description ??
                "Automatic fetching and preparation are healthy, nothing needs a decision, and every live channel has playable filler."}
            </p>
          </div>
          {action ? (
            action.to === "/channels/$id/filler" && readiness.channelId ? (
              <Button render={<Link to={action.to} params={{ id: readiness.channelId }} />}>
                {action.label}
              </Button>
            ) : action.to !== "/channels/$id/filler" ? (
              <Button render={<Link to={action.to} />}>{action.label}</Button>
            ) : null
          ) : null}
        </div>
      </Card>

      <section aria-labelledby="filler-flow-heading">
        <div className="mb-3">
          <h2 id="filler-flow-heading" className="font-semibold text-lg">
            What filler is doing
          </h2>
          <p className="text-muted-foreground text-sm">
            Machine work, decisions, and history are counted separately.
          </p>
        </div>
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <Card className="p-4">
            <p className="text-muted-foreground text-sm">Working automatically</p>
            <p className="mt-1 font-semibold text-2xl tabular-nums">{machineWork}</p>
            <p className="mt-1 text-muted-foreground text-xs">Queued, scheduled, or being prepared</p>
          </Card>
          <Card className="p-4">
            <p className="text-muted-foreground text-sm">Needs your decision</p>
            <p className="mt-1 font-semibold text-2xl tabular-nums">{readiness.pipeline.needsDecision}</p>
            <p className="mt-1 text-muted-foreground text-xs">
              Ambiguous items Loomarr will not file automatically
            </p>
          </Card>
          <Card className="p-4">
            <p className="text-muted-foreground text-sm">Recoverable</p>
            <p className="mt-1 font-semibold text-2xl tabular-nums">{readiness.pipeline.recoverable}</p>
            <p className="mt-1 text-muted-foreground text-xs">
              Failed work with an explicit retry or restore action
            </p>
          </Card>
          <Card className="p-4">
            <p className="text-muted-foreground text-sm">In the library</p>
            <p className="mt-1 font-semibold text-2xl tabular-nums">{readiness.pipeline.admitted}</p>
            <p className="mt-1 text-muted-foreground text-xs">
              Rejected or dismissed: {readiness.pipeline.rejected + readiness.pipeline.dismissed}
            </p>
          </Card>
        </div>
      </section>

      <section aria-labelledby="coverage-heading">
        <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="coverage-heading" className="font-semibold text-lg">
              Channel coverage
            </h2>
            <p className="text-muted-foreground text-sm">
              Playable time and grounded variety at each channel's widest eligible match.
            </p>
          </div>
          <Button variant="outline" size="sm" render={<Link to="/filler/library" />}>
            Browse library
          </Button>
        </div>
        {readiness.pool.channels.length > 0 ? (
          <div className="grid gap-3 lg:grid-cols-2">
            {readiness.pool.channels.map((channel) => (
              <Card key={channel.channelId} className="p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <Link
                      className="font-medium hover:underline"
                      to="/channels/$id/filler"
                      params={{ id: channel.channelId }}
                    >
                      {channel.number} · {channel.name}
                    </Link>
                    <p className="mt-1 text-muted-foreground text-sm">
                      {formatDuration(channel.durationMs)} playable · {pluralize(channel.total, "clip")}
                    </p>
                  </div>
                  <Badge
                    variant={
                      channel.level === "exact"
                        ? "signal"
                        : channel.level === "bumper_card"
                          ? "lock"
                          : "caution"
                    }
                  >
                    {channel.level === "bumper_card" ? "Bumper only" : channel.level.replace("_", " ")}
                  </Badge>
                </div>
                <p className="mt-3 text-muted-foreground text-xs">
                  {channel.categories} {channel.categories === 1 ? "category" : "categories"} ·{" "}
                  {pluralize(channel.brands, "brand")}
                </p>
              </Card>
            ))}
          </div>
        ) : (
          <Card className="p-4 text-muted-foreground text-sm">
            No live channels need filler coverage yet.
          </Card>
        )}
      </section>

      <section aria-labelledby="acquisition-heading">
        <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="acquisition-heading" className="font-semibold text-lg">
              Recent acquisitions
            </h2>
            <p className="text-muted-foreground text-sm">
              A trace from each fetch request through its admitted outcome.
            </p>
          </div>
          <Button variant="outline" size="sm" render={<Link to="/filler/sources" />}>
            Manage sources
          </Button>
        </div>
        {readiness.acquisitions.length > 0 ? (
          <div className="overflow-hidden rounded-lg border border-border">
            {readiness.acquisitions.map((run) => (
              <div
                key={run.id}
                className="flex flex-wrap items-center gap-3 border-border border-b p-3 last:border-b-0"
              >
                <Badge variant={statusTone(run.status)}>{run.status}</Badge>
                <div className="min-w-0 flex-1">
                  <p className="font-medium text-sm">
                    {run.trigger === "pull" ? "Approved pull" : `${run.trigger} acquisition`}
                  </p>
                  <p className="truncate text-muted-foreground text-xs">{acquisitionSummary(run)}</p>
                </div>
                <span className="text-muted-foreground text-xs">{formatRelative(run.updatedAt)}</span>
              </div>
            ))}
          </div>
        ) : (
          <Card className="p-4 text-muted-foreground text-sm">
            No acquisition runs have been recorded yet.
          </Card>
        )}
      </section>
    </div>
  );
};

export { FillerOverview, nextAction };
