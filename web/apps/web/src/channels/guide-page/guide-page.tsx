import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import { pluralize } from "@loomarr/core/format";
import {
  formatGuideTime,
  formatGuideTimeRange,
  type GuideAiringLayout,
  type GuideChannelLayout,
  type GuideLayout,
  type GuideNavigationDirection,
  type GuideSelection,
  guideSelectionForChannel,
  layoutGuide,
  moveGuideSelection,
  useGuideWindow,
} from "@loomarr/core/guide";
import {
  Action,
  Chip,
  CompactSelect,
  Notice,
  SegmentedControl,
  Stack,
  Surface,
  Text,
} from "@loomarr/design-system";
import {
  GuideCompact,
  type GuideFilter,
  GuideGrid,
  GuideNowNext,
  GuidePhoneHeader,
  GuideProgrammeDetail,
} from "@loomarr/ui";
import { keepPreviousData, type QueryClient, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ServedImage } from "@/components/loomarr/served-image";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { useDocumentTitle } from "@/lib/use-document-title";
import { usePhoneWidth } from "@/lib/use-phone-width";
import { useRouteContent } from "@/lib/use-route-content";
import { ChannelRowMenu } from "../channel-row-menu";
import {
  DEFAULT_WINDOW_MINUTES,
  guideDayOffset,
  guideDayStart,
  guideHourOf,
  guideWindow,
} from "../guide-window";
import { useGuideDay } from "../use-guide-day";

// The Guide — THE channels surface (§12), headed "Channels". One row per channel, time the
// shared axis. It answers both halves of the question in one place: "what do I have" and
// "what is on".
//
// It is also the door to **origination**: `✦ Add a channel` in the header and the "Dead air" empty
// state both go to the request page (/requests/new), where describe→approve lives under Requests.
// **Evolution** still lives on /channels/{id}; this page never re-originates.
//
// Answers what the per-channel Upcoming strip cannot: "what is on across ALL my channels right
// now?". Reads GET /v1/guide, which returns per-channel timelines with an explicit `kind` per
// block, so a programme, a commercial pod and a still-acquiring slot render as three different
// things rather than as one undifferentiated "gap".

// The grid is the shared `ui/guide` one (#1659, N7). The window always fits the grid's width, and
// the span picker below is how you see more or less time (it replaced zoom, #1659).

// How far a window may span. Kept as compact chips because this is the everyday view choice;
// the longer phrase remains in each chip's accessible name.
const WINDOW_CHOICES = [
  { value: "120", label: "2h", description: "2 hours" },
  { value: "240", label: "4h", description: "4 hours" },
  { value: "360", label: "6h", description: "6 hours" },
  { value: "720", label: "12h", description: "12 hours" },
] as const;

// How far back the day picker offers. Matches the server's `guide.retention_hours` default —
// past listings are RECOMPUTED from each channel's current lineup, so a distant past would be
// fiction rather than history. The server clamps regardless; this keeps the UI from offering
// a day it will not serve.
const RETENTION_DAYS = 1;
const FORWARD_DAYS = 6;

// The now-line advances CLIENT-SIDE (`useGuideWindow`'s `nowMs`). The browser already knows the
// wall clock, so pushing "time passed" over SSE would be pure waste — a frame per second per
// viewer to transmit something every client can compute. SSE is reserved for the genuine
// invalidation (a lineup changed).

// The hours the START picker offers. Every third hour is enough to land anywhere in the day
// in one click; a 24-entry list is a scroll, not a choice.
const START_HOURS = [0, 3, 6, 7, 8, 9, 12, 15, 18, 20, 21, 22] as const;

const hourLabel = (h: number) => {
  const suffix = h < 12 ? "AM" : "PM";
  const twelve = h % 12 === 0 ? 12 : h % 12;
  return `${twelve} ${suffix}`;
};

// "auto" follows now; the rest pin the window's start to that hour of the chosen day.
const START_HOUR_OPTIONS = [
  { label: "Now", value: "auto" },
  ...START_HOURS.map((h) => ({ label: hourLabel(h), value: String(h) })),
];

// The guide's two views (#2766 on phones, #2787 on desktops): a Now/Next list, or the full grid.
const GUIDE_VIEWS = [
  { label: "Now & next", value: "list" },
  { label: "Full grid", value: "grid" },
] as const;
type GuideView = (typeof GUIDE_VIEWS)[number]["value"];

// The selection survives a new window or a refetch: the same channel, at the block nearest the
// same time, as the shared guide controller does. Otherwise the block airing now on the first
// channel.
const reconcileSelection = (
  layout: GuideLayout | undefined,
  selection: GuideSelection | undefined,
  atMs: number,
): GuideSelection | undefined => {
  if (!layout) return undefined;
  const kept = selection && guideSelectionForChannel(layout, selection.channelId, selection.anchorMs);
  const first = layout.channels[0]?.source.channelId;
  return kept ?? (first ? guideSelectionForChannel(layout, first, atMs) : undefined);
};

// The programme card's art: the shared ServedImage (ThumbHash placeholder, sized sources),
// contained rather than cropped so it still identifies what will air.
const renderArtwork = (airing: GuideAiringLayout) =>
  airing.source.thumbImage ? (
    <ServedImage image={airing.source.thumbImage} alt="" sizes="360px" fit="contain" />
  ) : airing.source.thumbUrl ? (
    <img src={airing.source.thumbUrl} alt="" className="size-full object-contain" />
  ) : undefined;

// The day picker's offset of a moment from today on the household's calendar, within the days the
// picker offers.
const dayOffsetOf = (atMs: number, timeZone?: string): number =>
  Math.min(FORWARD_DAYS, Math.max(-RETENTION_DAYS, guideDayOffset(atMs, Date.now(), timeZone)));

// The day picker's label: relative for the days people actually mean, absolute beyond that, and
// the household's date, as the axis is.
const dayLabel = (offset: number, now: number, timeZone?: string): string => {
  if (offset === 0) return "Today";
  if (offset === 1) return "Tomorrow";
  if (offset === -1) return "Yesterday";
  return new Date(guideDayStart(now, offset, timeZone)).toLocaleDateString(undefined, {
    weekday: "short",
    month: "short",
    day: "numeric",
    timeZone,
  });
};

// The household's zone, which the axis is labelled in and its days and hours are read on. Every
// guide answer carries it, and the route's loader has fetched one before the page renders.
const cachedGuideZone = (queryClient: QueryClient): string | undefined =>
  queryClient
    .getQueriesData<channelsApi.channelGuideResponse>({ queryKey: channelsApi.getChannelGuideQueryKey() })
    .map(([, data]) => unwrap(data)?.timezone)
    .find((zone) => zone !== undefined && zone !== "");

const GuidePage = () => {
  // The tab is named for the nav item that opens it, as every other page's is.
  useDocumentTitle("Guide");
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { isAdmin } = useAuth();
  // Selection owns the grid's roving Tab stop; actual hover/focus owns its preview.
  // `?at=&channel=` (Home's Tonight rows, #2763) opens the grid on that day and hour with that
  // channel's programme selected. Read once, on mount: afterwards the controls own the window.
  const { view: viewParam, at: atParam, channel: channelParam } = useSearch({ from: "/_authed/guide" });
  const [selection, setSelection] = useState<GuideSelection | undefined>(() =>
    atParam !== undefined && channelParam ? { channelId: channelParam, anchorMs: atParam } : undefined,
  );
  const [timeZone, setTimeZone] = useState(() => cachedGuideZone(queryClient));
  // A phone opens on the shortest span: the compact grid (mocks 5d/5f) shows about an hour and a
  // half across, and four hours leave a half-hour programme a few pixels wide.
  const phoneWidth = usePhoneWidth();
  const [windowMinutes, setWindowMinutes] = useState<number>(() =>
    phoneWidth ? Number(WINDOW_CHOICES[0].value) : DEFAULT_WINDOW_MINUTES,
  );
  // `nowMs` drives the now-line and moves on its own timer. `anchorMs` pins TODAY's window so the
  // grid does not slide under the reader while they look at it: it advances only when the clock
  // nears the window's end (see `useGuideWindow`). The now-line moves; the axis stays put.
  // The guide opens on the Now/Next list, phone and desktop alike (#2787); `?view=grid` is the full
  // grid. The list always asks about now, and about a few hours so that "next" is usually listed;
  // the day, hour and span controls belong to the grid.
  const view: GuideView = viewParam === "grid" ? "grid" : "list";
  const setView = useCallback(
    (next: GuideView) =>
      void navigate({
        to: "/guide",
        search: (prev) => ({ ...prev, view: next === "grid" ? "grid" : undefined }),
        replace: true,
      }),
    [navigate],
  );
  const nowNextView = view === "list";
  const queryMinutes = nowNextView ? DEFAULT_WINDOW_MINUTES : windowMinutes;
  const { nowMs, anchorMs } = useGuideWindow(queryMinutes);
  const [dayOffset, setDayOffset] = useGuideDay({
    anchorMs,
    initialOffset: atParam === undefined ? 0 : dayOffsetOf(atParam, timeZone),
    minOffset: -RETENTION_DAYS,
    timeZone,
  });
  // Nudges the window by whole hours without changing the day — the ‹ › stepper. Kept separate
  // from `startHour` so stepping away from a chosen start hour is still relative to it.
  const [hourShift, setHourShift] = useState<number>(0);
  // null = "follow now" (the default). A number pins the window to that hour of the shown day,
  // which is how you ask "what airs at 7am?" without stepping there an hour at a time.
  const [startHour, setStartHour] = useState<number | null>(() =>
    atParam === undefined ? null : guideHourOf(atParam, timeZone),
  );
  // A precise start is a useful planning tool, but not a question most people ask on every
  // visit. It lives in a secondary row, and the trigger announces when a hidden non-default
  // remains active, so progressive disclosure never turns into invisible state.
  const [viewOptionsOpen, setViewOptionsOpen] = useState(false);
  const hasCustomView = startHour !== null;
  // Escape closes the panel, wherever focus is. An Escape that belongs to the open Start-hour list
  // (it is a portal, so its events reach here too) closes that list only.
  useEffect(() => {
    if (!viewOptionsOpen) return;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented) return;
      if (event.target instanceof Element && event.target.closest('[role="listbox"]')) return;
      setViewOptionsOpen(false);
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [viewOptionsOpen]);
  // The window lives in guide-window.ts, shared with the route's prefetch loader: TODAY's start
  // is quantised to a minute so a hover-prefetch and the click that follows address the SAME
  // react-query key. An unquantised window recomputes `from` between the two and warms an entry
  // the page never reads — a prefetch that costs a request and saves nothing.
  const { from, to } = useMemo(
    () =>
      nowNextView
        ? guideWindow({
            at: anchorMs,
            dayOffset: 0,
            windowMinutes: queryMinutes,
            hourShift: 0,
            startHour: null,
          })
        : guideWindow({ at: anchorMs, dayOffset, windowMinutes, hourShift, startHour, timeZone }),
    [nowNextView, dayOffset, queryMinutes, windowMinutes, anchorMs, hourShift, startHour, timeZone],
  );

  // "Are we looking at the present?" — drives the NOW button's tone. Any of the three
  // re-framing controls being off-default means no.
  const atNow = dayOffset === 0 && hourShift === 0 && startHour === null;

  // `keepPreviousData`: a stepper/day/span change is a NEW query key, and without placeholder
  // data `guide.data` goes undefined until the server answers — the grid empties and the toolbar
  // (gated on `channels.length`) unmounts, so the operator stares at a blank page for the full
  // server latency (#1396). The previous window stays on screen, flagged by `isPlaceholderData`.
  const guide = channelsApi.useChannelGuide(
    { from, to },
    { query: { retry: false, placeholderData: keepPreviousData } },
  );

  // Warm the windows one click away — an hour either side (the stepper) and tomorrow at the same
  // offset (the day picker's next stop) — once the CURRENT one has really loaded. Not while the
  // displayed data is still the previous window's placeholder, or a click would fan out requests
  // for windows the operator has already left. `prefetchQuery` honours the client's staleTime, so
  // settling on the same window twice does not refetch.
  const settled = guide.isSuccess && !guide.isPlaceholderData;
  useEffect(() => {
    if (!settled || nowNextView) return;
    const neighbours = [
      { hourShift: hourShift - 1, dayOffset },
      { hourShift: hourShift + 1, dayOffset },
      ...(dayOffset < FORWARD_DAYS ? [{ hourShift, dayOffset: dayOffset + 1 }] : []),
    ];
    for (const n of neighbours) {
      const w = guideWindow({ at: anchorMs, windowMinutes, startHour, timeZone, ...n });
      void queryClient.prefetchQuery(channelsApi.getChannelGuideQueryOptions(w));
    }
  }, [settled, nowNextView, queryClient, anchorMs, dayOffset, hourShift, windowMinutes, startHour, timeZone]);

  // Live update needs no listener here: a `channel` frame means a lineup actually changed (a
  // reconcile completed), and the SSE bus invalidates every cached `/v1/guide` window —
  // the one on screen and the prefetched neighbours alike. Deliberately not `title`: a title
  // becoming available fires BEFORE the backfill that places it, so refetching there would race
  // and often return the pre-backfill lineup.

  // The newest window that answered. TanStack v5 drops `placeholderData` once the query errors, so
  // a NEW window that fails (a span, a day, two hours on) has no data at all; the Guide the viewer
  // was just looking at stays on screen instead, under the notice, and only a page that has never
  // answered is given to the error.
  const answered = unwrap(guide.data);
  const [lastAnswered, setLastAnswered] = useState(answered);
  if (answered && answered !== lastAnswered) setLastAnswered(answered);
  const body = answered ?? lastAnswered;
  // The window on screen is the last good one, not the one that failed.
  const showingEarlierWindow = guide.error !== null && answered === undefined && body !== undefined;
  // A cold page with nothing cached reads the device's zone until the first answer names the
  // household's.
  useEffect(() => {
    if (body?.timezone) setTimeZone(body.timezone);
  }, [body?.timezone]);
  // Rows count as content from any window, the previous one included: that is what stays on
  // screen while the next window loads.
  useRouteContent(body !== undefined);
  const channels = useMemo(() => body?.channels ?? [], [body]);
  // "Known empty", not "nothing loaded yet": during the first fetch `channels` is also [],
  // and treating that as empty would flash the empty state (and hide the header button)
  // before the guide has answered.
  const isEmpty = channels.length === 0 && !guide.isLoading;

  // The now-line and "on now" belong to TODAY: another day's window never contains nowMs, so
  // the shared layout draws neither there.
  // Laid out only when there are channels to lay out: an empty guide shows Dead air, and a body
  // without `channels` (which `channels` above already reads defensively) must not reach it.
  const layout = useMemo(
    () => (body && channels.length > 0 ? layoutGuide(body, nowMs) : undefined),
    [body, channels, nowMs],
  );
  const selected = useMemo(() => reconcileSelection(layout, selection, nowMs), [layout, selection, nowMs]);

  // The phone grid's filters read the viewer's favourites and recents (#1778's lists), and the
  // Now/Next list puts the favourites first (#2787).
  const [filter, setFilter] = useState<GuideFilter>("all");
  const mine = channelsApi.useMyChannels({ query: { retry: false, enabled: phoneWidth || nowNextView } });
  const mineBody = unwrap(mine.data);
  const myChannels = useMemo(
    () =>
      mineBody && {
        favouriteIds: mineBody.favourites.map((f) => f.channelId),
        recentIds: mineBody.recent.map((r) => r.channelId),
      },
    [mineBody],
  );
  const onMove = useCallback(
    (direction: GuideNavigationDirection) => {
      if (!layout || !selected) return undefined;
      const moved = moveGuideSelection(layout, selected, direction);
      if (!moved.boundary) setSelection(moved.selection);
      return moved;
    },
    [layout, selected],
  );
  // A guide row or block tunes its channel (#2088): the channel page itself opens on Info, not Watch.
  const onOpenChannel = useCallback(
    (id: string) => void navigate({ to: "/watch/$id", params: { id } }),
    [navigate],
  );
  // A channel's name opens its page, where the row menu's other actions are one step away (#2745).
  const onOpenChannelInfo = useCallback(
    (id: string) => void navigate({ to: "/channels/$id", params: { id } }),
    [navigate],
  );
  const renderRowMenu = useCallback(
    (ch: GuideChannelLayout["source"]) => (
      <ChannelRowMenu channel={{ id: ch.channelId, name: ch.name, status: ch.status }} />
    ),
    [],
  );

  const dayChoices = useMemo(() => {
    const out: number[] = [];
    for (let d = -RETENTION_DAYS; d <= FORWARD_DAYS; d++) out.push(d);
    return out;
  }, []);

  // A failed refetch of a Guide already on screen (an SSE invalidation, a window revisited) keeps it
  // and says it may be stale; only a Guide that never answered gives the page to the error.
  if (guide.error && !body) {
    return (
      <div className="p-6">
        <ErrorState error={guide.error} onRetry={() => guide.refetch()} />
      </div>
    );
  }

  return (
    // No page padding and no gap: the mock's guide is a full-bleed surface whose header,
    // toolbar and grid are separated by RULES, not whitespace, and whose grid owns the only
    // scroll container. Padding here is what produced a page that scrolled horizontally
    // underneath a grid that had room.
    <div className="flex min-h-0 flex-1 flex-col">
      {/* Row 1 — title + the one origination door, far right (mock: its own row). */}
      <PageHeader
        title="Guide"
        description="Every channel Loomarr manages, and what's on right now."
        actions={
          /* THE origination door (§12) — for both roles. It goes to the request page
            (/requests/new), where the describe→approve flow lives under Requests.

            The LABEL carries the role difference, not the presence: an admin adds a channel
            (their approval is the last step), a member requests one (an admin approves it).
            Same flow, same call — §7 enforces the gate server-side either way.

            ⚠ HIDDEN ON AN EMPTY GUIDE, because the empty state below already offers this
            exact action. Two buttons a few hundred pixels apart read as two different
            features, which is precisely how it landed for the maintainer on a fresh install. */
          !isEmpty && (
            <Action
              accessibilityRole="link"
              icon="suggest"
              render={<Link to="/requests/new" />}
              tone="suggest"
            >
              {isAdmin ? "Add a channel" : "Request a channel"}
            </Action>
          )
        }
      />

      {guide.error ? (
        // The same inset the TV and phone Guide give it (GuideJourney).
        <Surface backgroundColor="transparent" borderWidth={0} padding="control">
          <Notice
            action={{ label: "Try again", onPress: () => void guide.refetch() }}
            body={
              showingEarlierWindow
                ? "That time didn't load. Showing the last one that did."
                : "Showing what Loomarr had a moment ago."
            }
            title="Couldn't refresh the Guide"
            tone="warn"
          />
        </Surface>
      ) : null}

      {/* Row 2 — the everyday guide questions stay visible: which day, where now is, and how
          much time is on screen. Precise planning controls live in the secondary View row. */}
      {channels.length > 0 && (
        <GuidePhoneHeader
          // The page is headed "Guide" already; the grid's row says only which hours it shows.
          title={nowNextView ? "What's on" : undefined}
          subtitle={
            nowNextView
              ? `${formatGuideTime(nowMs, body?.timezone)} · ${pluralize(channels.length, "channel")}`
              : showingEarlierWindow && body
                ? formatGuideTimeRange(body.fromMs, body.toMs, body.timezone)
                : formatGuideTimeRange(from, to, body?.timezone)
          }
          wide={!phoneWidth}
        >
          <SegmentedControl
            accessibilityLabel="Guide view"
            onValueChange={setView}
            options={GUIDE_VIEWS}
            value={view}
          />
        </GuidePhoneHeader>
      )}

      {channels.length > 0 && !nowNextView && (
        <>
          <div className="border-border/60 flex flex-wrap items-center gap-3.5 border-b px-7 py-2.5">
            <Stack align="center" direction="row" gap="1.75">
              <div>
                <CompactSelect
                  label="Day"
                  onValueChange={(v) => setDayOffset(Number(v))}
                  options={dayChoices.map((d) => ({
                    label: dayLabel(d, anchorMs, timeZone),
                    value: String(d),
                  }))}
                  value={String(dayOffset)}
                />
              </div>

              {/* A past day is RECOMPUTED from the current lineup, not a recording — and a
                  future one is a plan. The chip says which, and doubles as the way back. */}
              {dayOffset !== 0 && (
                <Chip mono onPress={() => setDayOffset(0)} tone={dayOffset < 0 ? "muted" : "info"}>
                  {`${dayOffset < 0 ? "AS AIRED" : "SCHEDULED"} · back to today`}
                </Chip>
              )}
            </Stack>

            {/* Hour stepper. NOW is the anchor an operator returns to; the arrows nudge the
                window without changing the day. */}
            <Stack align="center" direction="row" gap="1">
              <Action
                icon="earlier"
                layout="icon"
                onPress={() => setHourShift((h) => h - 1)}
                variant="outline"
              >
                Back an hour
              </Action>
              {/* Already at now, the anchor is the quiet choice; away from it, the way back. */}
              <Action
                onPress={() => {
                  setHourShift(0);
                  setDayOffset(0);
                  setStartHour(null);
                }}
                variant={atNow ? "ghost" : "outline"}
              >
                NOW
              </Action>
              <Action icon="later" layout="icon" onPress={() => setHourShift((h) => h + 1)} variant="outline">
                Forward an hour
              </Action>
            </Stack>

            {/* Span stays in the primary row: it changes the question the guide answers, and
                four compact choices are faster to scan than a select. */}
            <Stack align="center" className="sm:ml-auto" direction="row" gap="1.75">
              <Text as="span" textRole="metadata">
                Span
              </Text>
              <Stack direction="row" gap="1">
                {WINDOW_CHOICES.map((w) => {
                  const active = windowMinutes === Number(w.value);
                  return (
                    <Chip
                      accessibilityLabel={`Show ${w.description}`}
                      key={w.value}
                      mono
                      onPress={() => setWindowMinutes(Number(w.value))}
                      selected={active}
                    >
                      {w.label}
                    </Chip>
                  );
                })}
              </Stack>
            </Stack>

            <Action
              accessibilityLabel={hasCustomView ? "View options, custom" : "View options"}
              aria-controls="guide-view-options"
              aria-expanded={viewOptionsOpen}
              icon="filter"
              onPress={() => setViewOptionsOpen((open) => !open)}
              variant="outline"
            >
              {hasCustomView ? "View · custom" : "View"}
            </Action>
          </div>

          {viewOptionsOpen && (
            <div
              id="guide-view-options"
              className="border-border/60 bg-card/40 flex flex-wrap items-center justify-start gap-x-6 gap-y-2 border-b px-7 py-2.5 sm:justify-end"
            >
              {/* Pin the window to a specific part of the chosen day without stepping there
                  one hour at a time. */}
              <Stack align="center" direction="row" gap="1.75">
                <Text as="span" textRole="metadata">
                  Start at
                </Text>
                <div>
                  <CompactSelect
                    label="Start hour"
                    onValueChange={(v) => setStartHour(v === "auto" ? null : Number(v))}
                    options={START_HOUR_OPTIONS}
                    value={startHour === null ? "auto" : String(startHour)}
                  />
                </div>
              </Stack>
            </div>
          )}
        </>
      )}

      {isEmpty ? (
        // No channels yet is a real, expected state on a fresh install — not an error. The
        // one next action (§6) goes to the same request page the header links to, worded for
        // who is asking.
        //
        // ⚠ The LABEL matches the header's exactly, and the panel it opens is titled the same:
        // one action, one name, never a second wording such as "Describe your first channel".
        // ⚠ No static behind it (#2789): TvStatic is for entry moments only (login, the wizard
        // and the player's tune-in), so a resting page stays still.
        //
        // No scanline sweeps over the bars either: to register on a small element it has to be
        // scaled up and brightened, at which point it dominates a surface whose whole job is to
        // stay calm and point at one button. A sweep is an attention-grabber, and "Dead air" is a
        // RESTING state.
        <div className="flex flex-1 items-center justify-center p-10">
          <EmptyState
            // The state family's own static colour bars lead the panel (#2783): a test card is
            // what a set showed when nothing was broadcasting. Still, like the page around it.
            title="Dead air"
            description="No channels yet. Describe the channel you want and Loomarr builds the lineup."
            action={{
              label: isAdmin ? "Add a channel" : "Request a channel",
              onClick: () => void navigate({ to: "/requests/new" }),
            }}
          />
        </div>
      ) : (
        // Desktop browsing uses the full available width with contextual previews.
        <div className="relative flex min-h-0 flex-1">
          {/* The next window is still loading and the grid below is the PREVIOUS one. An
                overlaid hairline, not a toolbar addition, so the chrome never shifts. */}
          {guide.isPlaceholderData && (
            <div
              role="status"
              aria-label="Loading guide"
              className="bg-signal pointer-events-none absolute inset-x-0 top-0 z-30 h-0.5 animate-pulse"
            />
          )}
          {layout && nowNextView ? (
            // The guide opens on the Now/Next list (#2766, #2787, the Direction mock): one row per
            // channel, no time grid, favourites first. A row tunes its channel, a long-press or its ⋯
            // opens the channel's info, and the full grid is one tap away.
            <GuideNowNext
              favouriteIds={myChannels?.favouriteIds}
              layout={layout}
              nowMs={nowMs}
              onOpenChannelInfo={onOpenChannelInfo}
              onWatch={onOpenChannel}
              wide={!phoneWidth}
            />
          ) : layout && phoneWidth ? (
            // The phone's Full grid view: the native compact grid (#1785, mocks 5d/5f, decision N6): the number
            // column, the filter chips, and the tapped programme docked with Watch.
            <GuideCompact
              layout={layout}
              nowMs={nowMs}
              myChannels={myChannels}
              filter={filter}
              onFilterChange={setFilter}
              selection={selected}
              onSelect={setSelection}
              onWatch={(id) => void navigate({ to: "/watch/$id", params: { id } })}
              renderArtwork={renderArtwork}
            />
          ) : layout ? (
            <GuideGrid
              layout={layout}
              nowMs={nowMs}
              onMove={onMove}
              onOpenChannel={onOpenChannel}
              onOpenChannelInfo={onOpenChannelInfo}
              onSelect={setSelection}
              renderRowMenu={renderRowMenu}
              selection={selected}
              renderPreview={(previewSelection) => (
                <GuideProgrammeDetail
                  focused={false}
                  layout={layout}
                  renderArtwork={renderArtwork}
                  selection={previewSelection}
                />
              )}
            />
          ) : null}
        </div>
      )}
    </div>
  );
};

export { GuidePage };
