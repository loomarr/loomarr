import * as channelsApi from "@loomarr/api/endpoints/channels";
import * as dashboardApi from "@loomarr/api/endpoints/dashboard";
import * as titlesApi from "@loomarr/api/endpoints/titles";
import { unwrap } from "@loomarr/api/unwrap";
import { layoutGuide } from "@loomarr/core/guide";
import { Link } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { defaultGuideWindow } from "@/channels/guide-window";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { useDocumentTitle } from "@/lib/use-document-title";
import { ChannelIdeas } from "../channel-ideas";
import { HomeStrip } from "../home-strip";
import { NewThisWeek, newSince } from "../new-this-week";
import { OnNow } from "../on-now";
import { OnTheWay, YourRequests } from "../on-the-way";
import { RecentlyTuned } from "../recently-tuned";
import { Tonight } from "../tonight";
import { WatchingNow } from "../watching-now";

// Progress bars and "what's on now" move with the clock, not only with refetches.
const useMinuteClock = () => {
  const [nowMs, setNowMs] = useState(Date.now);
  useEffect(() => {
    const id = window.setInterval(() => setNowMs(Date.now()), 30_000);
    return () => window.clearInterval(id);
  }, []);
  return nowMs;
};

// HomePage — the landing page for both roles (#1822 evidence): a neutral inventory line, then
// what's on, in the approved order (Recently tuned, On now, Watching now, Tonight, New this week,
// role-specific requests/ideas, the admin footer). Machine state (encoders, services, activity)
// lives in Settings → This server (Q-H7), which the admin footer points to.
//
// Loading, zero-channels and initial-error are whole-page states (the record's state contract):
// nothing below HomeStrip renders alongside them, so one guide failure never shows a half page of
// sections quietly querying with no channels to join.
//
// Each later section renders from its own endpoint and hides when it has nothing, so one slow or
// failing source never blanks the rest. The guide query is the SAME quantised window the Guide
// reads (and `/` prefetches), so arriving here warms the Guide and vice versa.
const HomePage = () => {
  useDocumentTitle("Home");
  const { isAdmin, user } = useAuth();
  const nowMs = useMinuteClock();

  const guide = channelsApi.useChannelGuide(defaultGuideWindow(nowMs));
  const body = unwrap(guide.data);
  // Only the channels the body actually carries: a body without them reads as none, not a crash.
  const channels = body?.channels ?? [];
  const layout = useMemo(
    () => (body ? layoutGuide({ ...body, channels }, nowMs) : undefined),
    [body, channels, nowMs],
  );
  const loading = guide.isLoading;
  const errored = guide.isError && !loading;
  const hasLife = !loading && !errored && channels.length > 0;
  const stripState = loading ? "loading" : errored ? "error" : channels.length === 0 ? "empty" : "ok";

  // Players report what they're showing every 30 seconds (#1662), so poll at the same cadence.
  const viewing = dashboardApi.useHouseholdViewing({ query: { enabled: hasLife, refetchInterval: 30_000 } });
  const highlights = channelsApi.useGuideHighlights(undefined, { query: { enabled: hasLife } });
  const viewingBody = unwrap(viewing.data);
  const since = newSince(nowMs);
  const arrivals = titlesApi.useListTitles({ since }, { query: { enabled: hasLife } });
  const allChannels = channelsApi.useListChannels({ query: { enabled: hasLife } });
  // Channels an active own-viewing session already represents, so Recently tuned never duplicates
  // the choice Watching now already shows (#1822 evidence).
  const activeChannelIds = useMemo(
    () => new Set((viewingBody?.viewers ?? []).filter((v) => v.you).map((v) => v.channelId)),
    [viewingBody],
  );

  return (
    <div className="flex h-full flex-col">
      <PageHeader title="Home" />
      <div className="flex-1 overflow-auto">
        <div className="mx-auto flex max-w-[1120px] flex-col gap-8 p-6">
          <HomeStrip
            state={stripState}
            count={channels.length}
            isAdmin={isAdmin}
            onRetry={() => void guide.refetch()}
          />
          {hasLife && layout && (
            <RecentlyTuned
              continueWatching={viewingBody?.continueWatching}
              layout={layout}
              nowMs={nowMs}
              activeChannelIds={activeChannelIds}
            />
          )}
          {hasLife && layout && <OnNow layout={layout} nowMs={nowMs} />}
          {hasLife && layout && viewingBody && (
            <WatchingNow viewing={viewingBody} layout={layout} nowMs={nowMs} />
          )}
          {hasLife && (
            <Tonight
              highlights={unwrap(highlights.data, (b) => b.highlights) ?? []}
              timeZone={body?.timezone}
            />
          )}
          {hasLife && (
            <NewThisWeek
              titles={unwrap(arrivals.data, (b) => b.titles) ?? []}
              channels={unwrap(allChannels.data, (b) => b.channels) ?? []}
              since={since}
              viewerName={user?.name}
              timeZone={body?.timezone}
            />
          )}
          {/* Ideas need a library, not a channel, so a bare install offers them too (H4). */}
          {!loading && !errored && !isAdmin && <ChannelIdeas empty={channels.length === 0} nowMs={nowMs} />}
          {hasLife && !isAdmin && <YourRequests />}
          {hasLife && isAdmin && <OnTheWay />}
          {isAdmin && (
            <p className="m-0 text-static-400 text-xs">
              Encoder, storage and service details are in{" "}
              <Link to="/settings/system" className="text-signal hover:underline">
                Settings → This server
              </Link>
              .
            </p>
          )}
        </div>
      </div>
    </div>
  );
};

export { HomePage };
