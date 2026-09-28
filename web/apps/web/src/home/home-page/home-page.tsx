import * as channelsApi from "@loomarr/api/endpoints/channels";
import * as dashboardApi from "@loomarr/api/endpoints/dashboard";
import { unwrap } from "@loomarr/api/unwrap";
import { layoutGuide } from "@loomarr/core/guide";
import { Link } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { defaultGuideWindow } from "@/channels/guide-window";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { useDocumentTitle } from "@/lib/use-document-title";
import { HomeStrip } from "../home-strip";
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

// HomePage — the landing page for both roles (#1659 web mock, Q-H1): a status strip, then what's
// on. Only the variant sections differ by role; machine state (encoders, services, activity) lives
// in Settings → This server (Q-H7), which the admin footer points to.
//
// Each section renders from its own endpoint and hides when it has nothing, so one slow or
// failing source never blanks the page. The guide query is the SAME quantised window the Guide
// reads (and `/` prefetches), so arriving here warms the Guide and vice versa.
const HomePage = () => {
  useDocumentTitle("Home");
  const { isAdmin } = useAuth();
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
  const hasLife = !loading && channels.length > 0;

  // Players report what they're showing every 30 seconds (#1662), so poll at the same cadence.
  const viewing = dashboardApi.useHouseholdViewing({ query: { enabled: hasLife, refetchInterval: 30_000 } });
  const highlights = channelsApi.useGuideHighlights(undefined, { query: { enabled: hasLife } });
  const viewingBody = unwrap(viewing.data);

  return (
    <div className="flex h-full flex-col">
      <PageHeader title="Home" />
      <div className="flex-1 overflow-auto">
        <div className="mx-auto flex max-w-[1120px] flex-col gap-8 p-6">
          <HomeStrip channels={channels} loading={loading} isAdmin={isAdmin} />
          {hasLife && layout && viewingBody && (
            <WatchingNow viewing={viewingBody} layout={layout} nowMs={nowMs} />
          )}
          {hasLife && (
            <Tonight
              highlights={unwrap(highlights.data, (b) => b.highlights) ?? []}
              timeZone={body?.timezone}
            />
          )}
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
