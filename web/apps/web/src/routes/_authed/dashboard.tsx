import * as dashboardApi from "@loomarr/api/endpoints/dashboard";
import * as systemApi from "@loomarr/api/endpoints/system";
import type { Activity } from "@loomarr/api/models/activity";
import { unwrap } from "@loomarr/api/unwrap";
import { formatRelative } from "@loomarr/core/format";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { ActivityFeed } from "@/components/loomarr/dashboard/activity-feed";
import { PlayoutPanel } from "@/components/loomarr/dashboard/playout-panel";
import { RestartNeededBanner } from "@/components/loomarr/dashboard/restart-needed-banner";
import { ServiceControl } from "@/components/loomarr/dashboard/service-control";
import { ServicesPanel } from "@/components/loomarr/dashboard/services-panel";
import { StatCard } from "@/components/loomarr/dashboard/stat-card";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { useRestartWatchContext } from "@/dashboard/restart-watch-provider";
import { useDocumentTitle } from "@/lib/use-document-title";

// Dashboard (§12, V16) — "is everything alright?" in one screen.
//
// Four cards answering the questions an operator opens the app with, over the live transcoding
// panel. Each card LINKS to the surface it summarizes: a dashboard that only reports is a wall
// of numbers, one that routes is a way in — which is why the mock makes every card clickable.
//
// ADMIN ONLY. The content is machine state — encoder families, realtime speed, how close the
// box is to its channel ceiling — which §11 keeps to admins. A member who navigates here gets a
// short explanation, never a 403 wall: they did nothing wrong, and the API refusing them is an
// implementation detail they should not have to interpret (V16's gate).

const DashboardScreen = () => {
  useDocumentTitle("Dashboard");
  const { isAdmin } = useAuth();

  // The queries are all admin-gated server-side, so they are not even issued for a member —
  // firing them would put four 403s in the console on a screen we deliberately show instead.
  const enabled = isAdmin;

  // One compact read model replaces seven collection fetches. SSE invalidates it on title,
  // proposal, and channel changes; the poll and focus refetch close any lossy-stream gap.
  const summary = dashboardApi.useDashboardSummary({
    query: { enabled, refetchInterval: 60_000, refetchOnWindowFocus: true },
  });
  // The single live-playout picture (§9.1 V47): channels on air with their throughput + cold-start,
  // AND the GPU/LLM-VRAM contention header — one endpoint, one panel. SSE catches start/stop;
  // a 5s poll runs only while internal playout is active so moving telemetry cannot freeze.
  const playout = dashboardApi.useGetPlayoutStatus({
    query: {
      enabled,
      refetchOnWindowFocus: true,
      refetchInterval: (query) => (unwrap(query.state.data, (body) => body.running) ? 5_000 : false),
    },
  });
  // Restart control (§9.2, V13). One cost query drives the restart-needed banner AND the
  // confirm line, so the two can never disagree about what a restart would do.
  const navigate = useNavigate();
  const restartCost = systemApi.useSystemRestartCost({ query: { enabled, refetchOnWindowFocus: true } });
  // Services probes every 30s. Activity rides SSE for latency and keeps a slower 60s poll as
  // reconnect insurance; both revalidate on focus.
  const services = systemApi.useSystemServices({
    query: { enabled, refetchInterval: 30_000, refetchOnWindowFocus: true },
  });
  const activity = systemApi.useListActivity(
    { limit: 8 },
    { query: { enabled, refetchInterval: 60_000, refetchOnWindowFocus: true } },
  );
  const [serviceError, setServiceError] = useState<string | null>(null);
  const controlRef = useRef<HTMLDivElement>(null);
  // The SHARED watch (app shell). The overlay it drives covers the whole app, so an
  // operator who navigates away mid-restart still sees what is happening.
  const watch = useRestartWatchContext();

  const restart = systemApi.useSystemRestart({
    mutation: {
      onSuccess: () => {
        setServiceError(null);
        watch.begin();
      },
      onError: () => setServiceError("Couldn't restart. Loomarr is still running the old settings."),
    },
  });

  const openActivity = (entry: Activity) => {
    if (entry.kind === "channel" && entry.subjectId) {
      void navigate({ to: "/channels/$id", params: { id: entry.subjectId } });
      return;
    }
    if (entry.kind === "proposal") {
      void navigate({ to: "/queue", search: { tab: "history" } });
      return;
    }
    if (entry.kind === "title") {
      void navigate({ to: "/queue", search: { tab: "flight" } });
      return;
    }
    if (entry.kind === "user") {
      void navigate({ to: "/people" });
      return;
    }
    if (entry.kind === "filler") {
      void navigate({ to: "/filler" });
      return;
    }
    void navigate({ to: "/settings" });
  };

  if (!isAdmin) {
    return (
      <div className="p-6">
        <EmptyState
          title="The dashboard is for admins"
          description="It shows how the machine is doing: encoders, streams and what's waiting on someone. Your channels and requests are under Guide and My requests."
        />
      </div>
    );
  }

  const snapshot = unwrap(summary.data);
  const playoutStatus = unwrap(playout.data);
  const cost = unwrap(restartCost.data);

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title="Dashboard"
        actions={
          <p className="text-muted-foreground text-xs" aria-live="polite">
            {summary.error && snapshot
              ? `Refresh failed · last updated ${formatRelative(snapshot.generatedAt)}`
              : summary.error
                ? "Operational summary unavailable · retrying"
                : snapshot
                  ? `${summary.isFetching ? "Refreshing" : "Updated"} ${formatRelative(snapshot.generatedAt)}`
                  : "Loading operational summary…"}
          </p>
        }
      />

      {/* ⚠ `[&>*]:shrink-0` is load-bearing: the column is a flex-col that scrolls (overflow-auto).
          Without it, when the stacked panels are taller than the viewport, flex SHRINKS each panel
          below its content height — and every panel Card is `overflow-hidden` (for its rounded
          corners), so the shrunk height CLIPS its last rows (Filler/livetv vanished from Services).
          Pinning children to their natural height makes the COLUMN scroll instead of the panels
          clipping. This is the vertical twin of PanelRow's horizontal wrap fix. */}
      <div className="flex flex-1 flex-col gap-6 overflow-auto p-6 *:shrink-0">
        {/* Above the cards, because a setting that has not taken effect changes how every
            number below it should be read. The restart ITSELF is announced by the shell's
            overlay, which covers every page rather than just this one. */}
        <RestartNeededBanner
          pendingKeys={cost?.pendingKeys ?? []}
          onGoToRestart={() => controlRef.current?.scrollIntoView({ behavior: "smooth", block: "center" })}
        />

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Link to="/guide">
            <StatCard
              label="On air"
              value={snapshot?.onAir ?? (summary.error ? "!" : "—")}
              note={
                snapshot == null
                  ? "checking the guide"
                  : snapshot.channels === 0
                    ? "no channels configured"
                    : `of ${snapshot.channels} managed ${snapshot.channels === 1 ? "channel" : "channels"}`
              }
              tone="onair"
            />
          </Link>
          {/* Needs-you is the only card that is a CALL TO ACTION rather than a status, so it
              takes a colour only when the number is non-zero — a permanently pink zero would
              train the eye to ignore it. */}
          <Link to="/queue" search={{ tab: "approval" }}>
            <StatCard
              label="Needs you"
              value={snapshot?.needsApproval ?? (summary.error ? "!" : "—")}
              note={
                snapshot == null
                  ? "checking the approval queue"
                  : snapshot.needsApproval > 0
                    ? "requests waiting on approval"
                    : "nothing waiting"
              }
              tone={snapshot != null && snapshot.needsApproval > 0 ? "suggest" : "neutral"}
            />
          </Link>
          <Link to="/queue" search={{ tab: "flight" }}>
            <StatCard
              label="Acquiring"
              value={snapshot?.acquiring ?? (summary.error ? "!" : "—")}
              note="titles in flight"
              tone="tune"
            />
          </Link>
          <Link to="/queue" search={{ tab: "flight" }}>
            <StatCard
              label="Unavailable"
              value={snapshot?.unavailable ?? (summary.error ? "!" : "—")}
              note={snapshot?.unavailable ? "titles that need attention" : "no failed acquisitions"}
              tone={snapshot?.unavailable ? "onair" : "neutral"}
            />
          </Link>
        </div>

        {/* One live-playout picture (§9.1 V47): channels on air with throughput + cold-start, and the
            GPU/LLM-VRAM contention header. Merged from the former Transcoding + Playout-health panels,
            which duplicated channel id/speed/hardware per row. */}
        <PlayoutPanel
          status={playoutStatus}
          loading={playout.isLoading}
          error={playout.error ? "Playout status is unavailable. Retrying automatically." : undefined}
        />

        {services.data?.status === 200 ? (
          <ServicesPanel
            view={services.data.data}
            refreshing={services.isFetching}
            // "Fix →" routes to the settings that own the failing connection. A red dot
            // with nowhere to go is a puzzle, not a diagnosis (§12).
            onFix={(group) => navigate({ to: "/settings/connections", hash: group })}
          />
        ) : services.error ? (
          <ErrorState error={services.error} onRetry={() => services.refetch()} />
        ) : null}

        {activity.data?.status === 200 ? (
          <ActivityFeed entries={activity.data.data.activity ?? []} onOpen={openActivity} />
        ) : activity.error ? (
          <ErrorState error={activity.error} onRetry={() => activity.refetch()} />
        ) : null}

        {cost ? (
          <div ref={controlRef}>
            <ServiceControl
              cost={cost}
              onRestart={() => restart.mutate()}
              restarting={watch.restarting}
              // A restart that never came back is the failure worth naming — it outranks
              // a request-level error, because the app is actually down.
              error={watch.failed ?? serviceError}
            />
          </div>
        ) : null}
      </div>
    </div>
  );
};

const Route = createFileRoute("/_authed/dashboard")({
  component: DashboardScreen,
});

export { Route };
