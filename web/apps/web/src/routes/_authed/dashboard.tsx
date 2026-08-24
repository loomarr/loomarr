import * as channelsApi from "@loomarr/api/endpoints/channels";
import * as dashboardApi from "@loomarr/api/endpoints/dashboard";
import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import * as systemApi from "@loomarr/api/endpoints/system";
import * as titlesApi from "@loomarr/api/endpoints/titles";
import { TitleDTOState } from "@loomarr/api/models/titleDTOState";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueries } from "@tanstack/react-query";
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

  const channels = channelsApi.useListChannels({ query: { enabled } });
  // The single live-playout picture (§9.1 V47): channels on air with their throughput + cold-start,
  // AND the GPU/LLM-VRAM contention header — one endpoint, one panel. Refreshed by the `playout` SSE
  // frame (wired in @loomarr/core), so no refetchInterval here.
  const playout = dashboardApi.useGetPlayoutStatus({ query: { enabled } });
  // The approval queue's depth — the mock's `pendingCount`, the same number Queue's nav badge
  // and its "Needs approval" tab show. One source, so they cannot disagree.
  const pending = proposalsApi.useListProposals({ status: "submitted" }, { query: { enabled } });
  const pendingCount = unwrap(pending.data, (b) => b.proposals?.length) ?? 0;

  // "Acquiring" spans every non-available state, and GET /v1/titles filters by ONE state, so
  // this fans out and sums — the same aggregation the Queue page does.
  const acquiring = useQueries({
    queries: Object.values(TitleDTOState)
      .filter((s) => s !== TitleDTOState.available)
      .map((state) => ({ ...titlesApi.getListTitlesQueryOptions({ state }), enabled })),
  });
  const acquiringCount = acquiring.reduce((n, q) => n + (unwrap(q.data, (b) => b.titles?.length) ?? 0), 0);

  // The catalog's size comes from the WATCH endpoint, which counts it in SQL (§10 V51d).
  //
  // ⚠ This used to be an unbounded `useListFiller()` whose only use was `.length` — every column
  // of every clip fetched to render one number. Paging would have made it not merely wasteful but
  // WRONG: a 100-row default page reports "100 clips" on an install with a thousand. The same
  // endpoint backs the Filler page header, so the two cannot disagree.
  const fillerWatch = fillerApi.useFillerWatch({ query: { enabled } });

  // Restart control (§9.2, V13). One cost query drives the restart-needed banner AND the
  // confirm line, so the two can never disagree about what a restart would do.
  const navigate = useNavigate();
  const restartCost = systemApi.useSystemRestartCost({ query: { enabled } });
  // ⚠ Services POLLS (30s); the feed does NOT (§12). A probe result only exists because the
  // server went and asked, so pushing it would mean probing forever on an idle install. A
  // feed row IS an event the server knows at write time, so it rides the `activity` frame —
  // wired centrally in @loomarr/core, which is why there is no refetchInterval here.
  const services = systemApi.useSystemServices({ query: { enabled, refetchInterval: 30_000 } });
  const activity = systemApi.useListActivity({ limit: 8 }, { query: { enabled } });
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

  const rows = unwrap(channels.data, (b) => b.channels) ?? [];
  const onAir = rows.filter((c) => c.status === "live").length;
  const playoutStatus = unwrap(playout.data);
  const clipCount = unwrap(fillerWatch.data, (b) => b.clips) ?? 0;
  const cost = unwrap(restartCost.data);

  return (
    <div className="flex h-full flex-col">
      <PageHeader title="Dashboard" />

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

        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <Link to="/guide">
            <StatCard label="On air" value={onAir} note="channels in the guide right now" tone="onair" />
          </Link>
          {/* Needs-you is the only card that is a CALL TO ACTION rather than a status, so it
              takes a colour only when the number is non-zero — a permanently pink zero would
              train the eye to ignore it. */}
          <Link to="/queue" search={{ tab: "approval" }}>
            <StatCard
              label="Needs you"
              value={pendingCount}
              note={pendingCount > 0 ? "requests waiting on approval" : "nothing waiting"}
              tone={pendingCount > 0 ? "suggest" : "neutral"}
            />
          </Link>
          <Link to="/queue" search={{ tab: "flight" }}>
            <StatCard label="Acquiring" value={acquiringCount} note="titles in flight" tone="tune" />
          </Link>
          <Link to="/filler">
            <StatCard label="Filler" value={clipCount} note="clips in the shared catalog" tone="signal" />
          </Link>
        </div>

        {/* One live-playout picture (§9.1 V47): channels on air with throughput + cold-start, and the
            GPU/LLM-VRAM contention header. Merged from the former Transcoding + Playout-health panels,
            which duplicated channel id/speed/hardware per row. */}
        <PlayoutPanel status={playoutStatus} loading={playout.isLoading} />

        {services.data?.status === 200 ? (
          <ServicesPanel
            view={services.data.data}
            refreshing={services.isFetching}
            // "Fix →" routes to the settings that own the failing connection. A red dot
            // with nowhere to go is a puzzle, not a diagnosis (§12).
            onFix={(group) => navigate({ to: "/settings/connections", hash: group })}
            onDiagnose={(subsystem) =>
              navigate({
                to: "/settings/system/diagnostics",
                search: { view: "application", level: "error", subsystem },
              })
            }
          />
        ) : null}

        {activity.data?.status === 200 ? <ActivityFeed entries={activity.data.data.activity ?? []} /> : null}

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
