import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerReadinessDTO } from "@loomarr/api/models/fillerReadinessDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatDuration, pluralize } from "@loomarr/core/format";
import { Link } from "@tanstack/react-router";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";

type Action = {
  title: string;
  description: string;
  label: string;
  to: "/filler/incoming" | "/filler/library" | "/filler/manage" | "/filler/settings" | "/filler/sources";
};

// This maps the readiness projection's server-owned action enum to presentation only. Priority
// and whole-workspace health are never reconstructed from subsystem counts in the browser (§10).
const readinessAction = (readiness: FillerReadinessDTO): Action | undefined => {
  switch (readiness.nextAction) {
    case "none":
      return undefined;
    case "enable_fetch":
      return {
        title: "Turn on automatic sourcing",
        description: "Automatic source checks are off, so Loomarr cannot keep the filler catalog supplied.",
        label: "Review automation",
        to: "/filler/settings",
      };
    case "free_catalog_capacity":
      return {
        title: "Make room in the filler catalog",
        description: `Automatic sourcing paused at ${pluralize(readiness.actionCount ?? 0, "catalog clip")}. Remove clips or raise the limit to resume it.`,
        label: "Review limits",
        to: "/filler/settings",
      };
    case "free_disk_capacity":
      return {
        title: "Make room for more filler",
        description:
          "Automatic sourcing paused at its storage limit. Free space or raise the limit to resume it.",
        label: "Review limits",
        to: "/filler/settings",
      };
    case "retry_acquisition":
      return {
        title: "A source pull needs recovery",
        description: `${pluralize(readiness.actionCount ?? 0, "item")} could not finish entering the filler pipeline.`,
        label: "Open diagnostics",
        to: "/filler/manage",
      };
    case "retry_failed_work":
      return {
        title: "Some filler can be retried",
        description: `${pluralize(readiness.actionCount ?? 0, "item")} hit a recoverable processing problem.`,
        label: "Open diagnostics",
        to: "/filler/manage",
      };
    case "review_incoming":
      return {
        title: "A few clips need your judgment",
        description: `${pluralize(readiness.actionCount ?? 0, "clip")} could not be classified safely without a person.`,
        label: "Review clips",
        to: "/filler/incoming",
      };
    case "add_filler":
      return {
        title: "Add filler to get started",
        description: "No playable filler is available yet. Add a source or drop in your own clips.",
        label: "Open sources",
        to: "/filler/sources",
      };
    case "improve_channel_coverage": {
      const channel = readiness.pool.channels.find(
        (candidate) => candidate.channelId === readiness.channelId,
      );
      return {
        title: "A channel needs better filler coverage",
        description: channel
          ? `${channel.number} · ${channel.name} has a playable pool, but its coverage is still thin.`
          : "At least one channel has a playable pool, but its coverage is still thin.",
        label: "Browse library",
        to: "/filler/library",
      };
    }
    default: {
      const unhandled: never = readiness.nextAction;
      return unhandled;
    }
  }
};

const FillerOverview = () => {
  const decisionQuery = fillerApi.useFillerDecisionOverview();
  const readinessQuery = fillerApi.useFillerReadiness();
  const overview = unwrap(decisionQuery.data, (body) => body);
  const readiness = unwrap(readinessQuery.data, (body) => body);

  if (readinessQuery.error) {
    return <ErrorState error={readinessQuery.error} onRetry={() => readinessQuery.refetch()} />;
  }
  if (!readiness) {
    return (
      <Card aria-live="polite" className="p-6">
        <p className="font-medium">Checking filler health…</p>
        <p className="mt-1 text-muted-foreground text-sm">Reading the workspace readiness projection.</p>
      </Card>
    );
  }

  const action = readinessAction(readiness);

  return (
    <div className="flex flex-col gap-6">
      <Card className={readiness.ready ? "border-signal/35 p-5" : "border-caution/40 p-5"}>
        <div className="flex flex-col items-start gap-4 sm:flex-row">
          <div className="min-w-0 flex-1">
            <Badge variant={readiness.ready ? "signal" : "caution"}>
              {readiness.ready ? "Working automatically" : "Action recommended"}
            </Badge>
            <h2 className="mt-3 font-semibold text-xl">{action?.title ?? "Filler is working on its own"}</h2>
            <p className="mt-1 max-w-3xl text-muted-foreground text-sm">
              {action?.description ??
                "Recent clips were admitted or rejected automatically, and nothing needs your attention."}
            </p>
          </div>
          {action ? <Button render={<Link to={action.to} />}>{action.label}</Button> : null}
        </div>
      </Card>

      <section aria-labelledby="admission-summary-heading">
        <div className="mb-3">
          <h2 id="admission-summary-heading" className="font-semibold text-lg">
            Admission summary
          </h2>
          <p className="text-muted-foreground text-sm">Current outcomes, kept separate by the server.</p>
        </div>
        {decisionQuery.error ? (
          <ErrorState error={decisionQuery.error} onRetry={() => decisionQuery.refetch()} />
        ) : overview ? (
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <Card className="p-4">
              <p className="text-muted-foreground text-sm">Admitted</p>
              <p className="mt-1 font-semibold text-2xl tabular-nums">{overview.counts.admitted}</p>
              <p className="mt-1 text-muted-foreground text-xs">Ready for the compatible filing gate</p>
            </Card>
            <Card className="p-4">
              <p className="text-muted-foreground text-sm">Rejected automatically</p>
              <p className="mt-1 font-semibold text-2xl tabular-nums">{overview.counts.rejected}</p>
              <p className="mt-1 text-muted-foreground text-xs">Normal outcomes, not chores</p>
            </Card>
            <Card className="p-4">
              <p className="text-muted-foreground text-sm">Needs judgment</p>
              <p className="mt-1 font-semibold text-2xl tabular-nums">{overview.counts.unresolvedReviews}</p>
              <p className="mt-1 text-muted-foreground text-xs">Semantic questions only</p>
            </Card>
            <Card className="p-4">
              <p className="text-muted-foreground text-sm">Processing holds</p>
              <p className="mt-1 font-semibold text-2xl tabular-nums">{overview.counts.operational}</p>
              <p className="mt-1 text-muted-foreground text-xs">
                {pluralize(overview.counts.retryable, "retryable item")}
              </p>
            </Card>
          </div>
        ) : (
          <Card aria-live="polite" className="p-4 text-muted-foreground text-sm">
            Loading admission outcomes…
          </Card>
        )}
      </section>

      <section aria-labelledby="coverage-heading">
        <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="coverage-heading" className="font-semibold text-lg">
              Channel coverage
            </h2>
            <p className="text-muted-foreground text-sm">
              Playable time and variety remain separate from admission health.
            </p>
          </div>
          <Button variant="outline" size="sm" render={<Link to="/filler/library" />}>
            Browse library
          </Button>
        </div>
        {readiness?.pool.channels.length ? (
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
            {readinessQuery.error
              ? "Channel coverage is temporarily unavailable."
              : "No live channels need filler coverage yet."}
          </Card>
        )}
      </section>
    </div>
  );
};

export { FillerOverview, readinessAction };
