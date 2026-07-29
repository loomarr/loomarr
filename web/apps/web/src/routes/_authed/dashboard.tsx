import { channelsApi, dashboardApi, fillerApi, suggestionsApi, TitleDTOState, titlesApi } from "@loomarr/api";
import { useQueries } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { useAuth } from "@/auth";
import { EmptyState, StatCard, TranscodingPanel } from "@/components/loomarr";
import { useDocumentTitle } from "@/lib";

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
  const playout = dashboardApi.useGetPlayoutTelemetry({ query: { enabled } });
  // The approval queue's depth — the mock's `pendingCount`, the same number Queue's nav badge
  // and its "Needs approval" tab show. One source, so they cannot disagree.
  const pending = suggestionsApi.useListProposals({ status: "submitted" }, { query: { enabled } });
  const pendingCount = pending.data?.status === 200 ? (pending.data.data.proposals?.length ?? 0) : 0;

  // "Acquiring" spans every non-available state, and GET /v1/titles filters by ONE state, so
  // this fans out and sums — the same aggregation the Queue page does.
  const acquiring = useQueries({
    queries: Object.values(TitleDTOState)
      .filter((s) => s !== TitleDTOState.available)
      .map((state) => ({ ...titlesApi.getListTitlesQueryOptions({ state }), enabled })),
  });
  const acquiringCount = acquiring.reduce(
    (n, q) => n + (q.data?.status === 200 ? (q.data.data.titles?.length ?? 0) : 0),
    0,
  );

  const clips = fillerApi.useListFiller(undefined, { query: { enabled } });

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

  const rows = channels.data?.status === 200 ? (channels.data.data.channels ?? []) : [];
  const onAir = rows.filter((c) => c.status === "live").length;
  const telemetry = playout.data?.status === 200 ? playout.data.data : undefined;
  const clipCount = clips.data?.status === 200 ? (clips.data.data.clips?.length ?? 0) : 0;

  return (
    <div className="flex h-full flex-col">
      <header className="border-border border-b px-6 py-4">
        <h1 className="font-semibold text-xl">Dashboard</h1>
      </header>

      <div className="flex flex-1 flex-col gap-6 overflow-auto p-6">
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

        <TranscodingPanel telemetry={telemetry} loading={playout.isLoading} />
      </div>
    </div>
  );
};

const Route = createFileRoute("/_authed/dashboard")({
  component: DashboardScreen,
});

export { Route };
