import * as channelsApi from "@loomarr/api/endpoints/channels";
import * as dashboardApi from "@loomarr/api/endpoints/dashboard";
import * as titlesApi from "@loomarr/api/endpoints/titles";
import { unwrap } from "@loomarr/api/unwrap";
import { layoutGuide, useGuideWindow } from "@loomarr/core/guide";
import { keepPreviousData } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useMemo } from "react";
import { useAuth } from "@/auth/use-auth";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { useDocumentTitle } from "@/lib/use-document-title";
import { ChannelIdeas } from "../channel-ideas";
import { ChannelWall } from "../channel-wall";
import { HomeStrip } from "../home-strip";
import { useNeedsYou } from "../needs-you";
import { NewThisWeek, newSince } from "../new-this-week";
import { OnTheWay, YourRequests } from "../on-the-way";
import { Tonight } from "../tonight";
import { WatchingNow } from "../watching-now";

// HomePage — the landing page for both roles (#2764): it opens on the channel wall, every channel as
// a live tile with the last-tuned one first and marked (an admin's "Needs you" line above the tiles),
// then Watching now, Tonight, New this week, role-specific requests/ideas and the admin footer.
// Machine state (encoders, services, activity)
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
  // The window is pinned and slides only near its end; `nowMs` is the live clock that moves
  // progress bars and "what's on now" between refetches. `keepPreviousData` makes a slide refetch
  // behind the sections already on screen instead of unmounting them (the #1396 blank page).
  const { nowMs, window: guideWindow } = useGuideWindow();

  const guide = channelsApi.useChannelGuide(guideWindow, { query: { placeholderData: keepPreviousData } });
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
  const needs = useNeedsYou(isAdmin && hasLife);

  return (
    <div className="flex h-full flex-col">
      <PageHeader title="Home" />
      <div className="flex-1">
        {/* 16px padding at phone, 24px from `sm` up (the geometry table's "16px padding"). Below
            `md` the bottom bar replaced the 56px rail the table assumed, so 390px leaves 358px. */}
        <div className="max-w-overview mx-auto flex flex-col gap-6 p-4 sm:gap-8 sm:p-6">
          <HomeStrip state={stripState} isAdmin={isAdmin} onRetry={() => void guide.refetch()} />
          {hasLife && layout && (
            <ChannelWall
              lastChannelId={viewingBody?.continueWatching?.channelId}
              layout={layout}
              needs={needs}
              nowMs={nowMs}
            />
          )}
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
            <p className="text-static-400 m-0 text-xs">
              Encoder, storage and service details are in{" "}
              <Link to="/settings/system" className="text-tune underline underline-offset-2">
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
