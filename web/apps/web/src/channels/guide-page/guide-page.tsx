import { channelsApi, type GuideAiring } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";
import { EmptyState, ErrorState, GuideDetailCard, GuideGrid } from "@/components/loomarr";
import { Button } from "@/components/ui";
import { useLoomarrEventListener } from "@/events";

// The Guide — the cross-channel schedule (§12). One row per channel, time the shared axis.
//
// Answers what the per-channel Upcoming strip cannot: "what is on across ALL my channels
// right now?". Reads GET /v1/guide, which returns per-channel timelines with an explicit
// `kind` per block, so a programme, a commercial pod and a still-acquiring slot render as
// three different things rather than as one undifferentiated "gap".

// The zoom ladder, in pixels per minute. Zoom is ONE number — the grid scales everything
// against it — so these are just stops on that scale rather than distinct layouts.
// The zoom ladder, matching the mock's stops. Zoom scales the CHROME (rail, row height, type),
// never the time scale — the window always fits, and you change how much detail each row shows.
const ZOOM_STOPS = [0.75, 0.875, 1, 1.15, 1.35, 1.6] as const;
const DEFAULT_ZOOM_INDEX = 2;

// How much of the window sits behind "now". A guide opens on the present, but a programme
// already in progress needs its real start on screen or it appears to begin the moment you
// opened the page.
const LOOKBACK_MINUTES = 30;
const WINDOW_MINUTES = 4 * 60;

// The now-line advances CLIENT-SIDE from the block timestamps. The browser already knows
// the wall clock, so pushing "time passed" over SSE would be pure waste — one frame per
// second per viewer to transmit something every client can compute. SSE is reserved for
// the genuine invalidation (a lineup changed), below.
const NOW_TICK_MS = 30_000;

const GuidePage = () => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [zoomIndex, setZoomIndex] = useState<number>(DEFAULT_ZOOM_INDEX);
  const [nowMs, setNowMs] = useState<number>(() => Date.now());
  const [inspected, setInspected] = useState<GuideAiring | null>(null);

  useEffect(() => {
    const id = setInterval(() => setNowMs(Date.now()), NOW_TICK_MS);
    return () => clearInterval(id);
  }, []);

  // The window is pinned at mount rather than recomputed from `nowMs`, so the grid does not
  // slide under the reader every thirty seconds while they are looking at it. The now-line
  // moves; the axis stays put until an explicit refresh or a lineup change.
  const [windowStart] = useState<number>(() => Date.now() - LOOKBACK_MINUTES * 60_000);
  const from = windowStart;
  const to = windowStart + WINDOW_MINUTES * 60_000;

  const guide = channelsApi.useChannelGuide({ from, to }, { query: { retry: false } });

  // Live update: a `channel` frame means a lineup actually changed (a reconcile completed),
  // which is the ONE thing the client cannot derive on its own. Refetching on that keeps
  // the grid honest without polling.
  //
  // Deliberately not `onTitle`: a title becoming available fires before the backfill that
  // places it, so refetching there would race and often return the pre-backfill lineup.
  useLoomarrEventListener({
    onChannel: () => {
      void queryClient.invalidateQueries({ queryKey: channelsApi.getChannelGuideQueryKey({ from, to }) });
    },
  });

  const body = guide.data?.status === 200 ? guide.data.data : undefined;
  const channels = useMemo(() => body?.channels ?? [], [body]);

  if (guide.error) {
    return (
      <div className="p-6">
        <ErrorState error={guide.error} onRetry={() => guide.refetch()} />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4 p-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="font-semibold text-xl">Guide</h1>
          <p className="text-muted-foreground text-sm">What's on across every channel.</p>
        </div>
        <div className="flex items-center gap-1">
          <Button
            variant="outline"
            size="sm"
            aria-label="Zoom out"
            disabled={zoomIndex === 0}
            onClick={() => setZoomIndex((i) => Math.max(0, i - 1))}
          >
            −
          </Button>
          <Button
            variant="outline"
            size="sm"
            aria-label="Zoom in"
            disabled={zoomIndex === ZOOM_STOPS.length - 1}
            onClick={() => setZoomIndex((i) => Math.min(ZOOM_STOPS.length - 1, i + 1))}
          >
            +
          </Button>
        </div>
      </div>

      {channels.length === 0 && !guide.isLoading ? (
        // No channels yet is a real, expected state on a fresh install — not an error.
        <EmptyState
          title="No channels yet"
          description="Once you create a channel, its schedule shows up here."
        />
      ) : (
        // The detail card floats over the grid rather than displacing it: inspecting a block
        // must not reflow the schedule under the pointer.
        <div className="relative flex min-h-0 flex-1">
          <GuideGrid
            channels={channels}
            fromMs={body?.fromMs ?? from}
            toMs={body?.toMs ?? to}
            zoom={ZOOM_STOPS[zoomIndex]}
            nowMs={nowMs}
            onInspect={setInspected}
            onSelectChannel={(id) => navigate({ to: "/channels/$id", params: { id } })}
          />
          {inspected && (
            <div className="pointer-events-none absolute right-4 bottom-4 z-40">
              <GuideDetailCard airing={inspected} />
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export { GuidePage };
