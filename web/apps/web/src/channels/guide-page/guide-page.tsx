import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import {
  type GuideAiringLayout,
  type GuideChannelLayout,
  type GuideLayout,
  type GuideNavigationDirection,
  type GuideSelection,
  guideSelectionForChannel,
  layoutGuide,
  moveGuideSelection,
} from "@loomarr/core/guide";
import { AdaptiveSplit, adaptiveBreakpoints } from "@loomarr/design-system";
import { GuideGrid, GuideProgrammeDetail } from "@loomarr/ui";
import { keepPreviousData, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { SlidersHorizontal, Sparkles, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ColorBars } from "@/components/loomarr/shell/color-bars";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { TvStatic } from "@/components/loomarr/shell/tv-static";
import { Button } from "@/components/ui/button";
import { Caption } from "@/components/ui/caption";
import { Image } from "@/components/ui/image";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useLoomarrEventListener } from "@/events/events-provider";
import { cn } from "@/lib/utils";
import { type ChannelSuggestionStage, ChannelSuggestPanel } from "@/suggest/channel-suggest-panel";
import {
  clearSuggestionDraft,
  readActiveSuggestionJob,
  readSuggestionDraft,
} from "@/suggest/suggestion-draft";
import { ChannelRowMenu } from "../channel-row-menu";
import { DEFAULT_WINDOW_MINUTES, guideWindow } from "../guide-window";
import type { GuidePageProps } from "./guide-page.type";

// The Guide — THE channels surface (§12), headed "Channels". One row per channel, time the
// shared axis. It answers both halves of the question the old card list and this grid used to
// split between them: "what do I have" and "what is on".
//
// It also owns **origination**: `✦ Add a channel` in the header opens the inline describe→
// approve panel, and the "Dead air" empty state opens the same one. That affordance is why the
// card list could fold away — removing it earlier would have stranded the everyday way a
// channel gets made. **Evolution** still lives on /channels/{id}; this page never re-originates.
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

// The now-line advances CLIENT-SIDE from the block timestamps. The browser already knows the
// wall clock, so pushing "time passed" over SSE would be pure waste — a frame per second per
// viewer to transmit something every client can compute. SSE is reserved for the genuine
// invalidation (a lineup changed), below.
const NOW_TICK_MS = 30_000;

// The hours the START picker offers. Every third hour is enough to land anywhere in the day
// in one click; a 24-entry list is a scroll, not a choice.
const START_HOURS = [0, 3, 6, 7, 8, 9, 12, 15, 18, 20, 21, 22] as const;

const hourLabel = (h: number) => {
  const suffix = h < 12 ? "AM" : "PM";
  const twelve = h % 12 === 0 ? 12 : h % 12;
  return `${twelve} ${suffix}`;
};

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

// The programme card's art: the same Image (ThumbHash placeholder, sized sources) the old
// detail card used, contained rather than cropped so it still identifies what will air.
const renderArtwork = (airing: GuideAiringLayout) =>
  airing.source.thumbImage ? (
    <Image image={airing.source.thumbImage} alt="" sizes="360px" className="size-full object-contain" />
  ) : airing.source.thumbUrl ? (
    <img src={airing.source.thumbUrl} alt="" className="size-full object-contain" />
  ) : undefined;

// AdaptiveSplit measures the window, but this page sits beside the shell's sidebar (md:w-56). The
// card moves below the grid where the pointer guide's would, measured from the content area, so a
// 900px window stacks instead of squeezing the grid to a sliver.
const SHELL_SIDEBAR_PX = 224;
const SPLIT_BREAKPOINT = adaptiveBreakpoints.pointer + SHELL_SIDEBAR_PX;

const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

// The day picker's label: relative for the days people actually mean, absolute beyond that.
const dayLabel = (offset: number, now: number): string => {
  if (offset === 0) return "Today";
  if (offset === 1) return "Tomorrow";
  if (offset === -1) return "Yesterday";
  const d = new Date(startOfDay(new Date(now)) + offset * 86_400_000);
  return d.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" });
};

const GuidePage = ({ initialIntent, initialJobId, openOnArrival }: GuidePageProps) => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { isAdmin } = useAuth();
  const [nowMs, setNowMs] = useState<number>(() => Date.now());
  // The keyboard's block (the grid's one Tab stop) and the block under the pointer. The
  // programme card beside the grid shows the hovered one, else the selected one.
  const [selection, setSelection] = useState<GuideSelection>();
  const [hovered, setHovered] = useState<GuideSelection>();
  const [dayOffset, setDayOffset] = useState<number>(0);
  const [windowMinutes, setWindowMinutes] = useState<number>(DEFAULT_WINDOW_MINUTES);
  // Nudges the window by whole hours without changing the day — the ‹ › stepper. Kept separate
  // from `startHour` so stepping away from a chosen start hour is still relative to it.
  const [hourShift, setHourShift] = useState<number>(0);
  // null = "follow now" (the default). A number pins the window to that hour of the shown day,
  // which is how you ask "what airs at 7am?" without stepping there an hour at a time.
  const [startHour, setStartHour] = useState<number | null>(null);
  // A precise start is a useful planning tool, but not a question most people ask on every
  // visit. It lives in a secondary row, and the trigger announces when a hidden non-default
  // remains active, so progressive disclosure never turns into invisible state.
  const [viewOptionsOpen, setViewOptionsOpen] = useState(false);
  const hasCustomView = startHour !== null;
  // The inline "describe a channel" surface. Creating a channel IS describing one (Suggest),
  // so the create path is the ChannelSuggestPanel expanded in place — no separate empty-shell
  // dialog. `adding` toggles it open.
  //
  // Opens on arrival when the wizard handed off a template (§13), a setup round trip retained
  // a draft, or this tab owns an active Proposal Job. The last case makes a reload return to the
  // current review rather than a bare grid; closing still hides it for the current page lifetime.
  const [adding, setAdding] = useState(() =>
    Boolean(
      initialIntent || initialJobId || openOnArrival || readActiveSuggestionJob() || readSuggestionDraft(),
    ),
  );
  const [suggestionStage, setSuggestionStage] = useState<ChannelSuggestionStage>("describe");

  // Closing also CLEARS `?intent=`. Leaving it would make a refresh silently re-open the
  // panel with a template the operator already dismissed — and right after the wizard, on a
  // first-run install, a refresh is likely rather than exotic. `replace: true` keeps the
  // dismissal out of history: closing a panel is not a place you navigate back to.
  const closePanel = () => {
    setAdding(false);
    clearSuggestionDraft();
    if (initialIntent || initialJobId || openOnArrival)
      void navigate({ to: "/guide", search: {}, replace: true });
  };

  // A recovered job is deliberately resumable from the URL, but a user who explicitly starts
  // another channel must not resume that old job on reload. Keep the panel open while replacing
  // only this handoff's search fields; route-owned search remains free to retain any unrelated
  // parameters it grows in future.
  const startFresh = () => {
    setAdding(true);
    void navigate({
      to: "/guide",
      search: (search) => ({ ...search, intent: undefined, job: undefined }),
      replace: true,
    });
  };

  // The inline panel approved a proposal, which created the channel — drop the operator on
  // its page, and refresh the guide behind them.
  const onCreated = (id: string) => {
    setAdding(false);
    void queryClient.invalidateQueries({ queryKey: channelsApi.getListChannelsQueryKey() });
    // Navigating away to the new channel drops `?intent=` with the rest of the URL, so this
    // needs no explicit clear the way closePanel does.
    void navigate({ to: "/channels/$id", params: { id } });
  };

  useEffect(() => {
    const id = setInterval(() => setNowMs(Date.now()), NOW_TICK_MS);
    return () => clearInterval(id);
  }, []);

  // `mountedAt` pins TODAY's window so the grid does not slide under the reader every thirty
  // seconds while they are looking at it. The now-line moves; the axis stays put.
  const [mountedAt] = useState<number>(() => Date.now());

  // The window lives in guide-window.ts, shared with the route's prefetch loader: TODAY's start
  // is quantised to a minute so a hover-prefetch and the click that follows address the SAME
  // react-query key. An unquantised window recomputes `from` between the two and warms an entry
  // the page never reads — a prefetch that costs a request and saves nothing.
  const { from, to } = useMemo(
    () => guideWindow({ at: mountedAt, dayOffset, windowMinutes, hourShift, startHour }),
    [dayOffset, windowMinutes, mountedAt, hourShift, startHour],
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
    if (!settled) return;
    const neighbours = [
      { hourShift: hourShift - 1, dayOffset },
      { hourShift: hourShift + 1, dayOffset },
      ...(dayOffset < FORWARD_DAYS ? [{ hourShift, dayOffset: dayOffset + 1 }] : []),
    ];
    for (const n of neighbours) {
      const w = guideWindow({ at: mountedAt, windowMinutes, startHour, ...n });
      void queryClient.prefetchQuery(channelsApi.getChannelGuideQueryOptions(w));
    }
  }, [settled, queryClient, mountedAt, dayOffset, hourShift, windowMinutes, startHour]);

  // Live update: a `channel` frame means a lineup actually changed (a reconcile completed),
  // which is the ONE thing the client cannot derive on its own. Refetching on that keeps the
  // grid honest without polling.
  //
  // Deliberately not `onTitle`: a title becoming available fires BEFORE the backfill that
  // places it, so refetching there would race and often return the pre-backfill lineup.
  useLoomarrEventListener({
    onChannel: () => {
      void queryClient.invalidateQueries({ queryKey: channelsApi.getChannelGuideQueryKey({ from, to }) });
    },
  });

  const body = unwrap(guide.data);
  const channels = useMemo(() => body?.channels ?? [], [body]);
  // "Known empty", not "nothing loaded yet": during the first fetch `channels` is also [],
  // and treating that as empty would flash the empty state (and hide the header button)
  // before the guide has answered.
  const isEmpty = channels.length === 0 && !guide.isLoading;

  // The now-line and "on now" belong to TODAY: another day's window never contains nowMs, so
  // the shared layout draws neither there.
  const layout = useMemo(() => (body ? layoutGuide(body, nowMs) : undefined), [body, nowMs]);
  const selected = useMemo(() => reconcileSelection(layout, selection, nowMs), [layout, selection, nowMs]);
  const onMove = useCallback(
    (direction: GuideNavigationDirection) => {
      if (!layout || !selected) return undefined;
      const moved = moveGuideSelection(layout, selected, direction);
      if (!moved.boundary) setSelection(moved.selection);
      return moved;
    },
    [layout, selected],
  );
  const onOpenChannel = useCallback(
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

  if (guide.error) {
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
        title={
          !adding
            ? "Channels"
            : suggestionStage === "review"
              ? "Review your channel"
              : suggestionStage === "updating"
                ? "Updating suggestions"
                : suggestionStage === "generating"
                  ? "Finding titles"
                  : suggestionStage === "failed"
                    ? "Channel suggestion"
                    : isAdmin
                      ? "Add a channel"
                      : "Request a channel"
        }
        description={
          adding
            ? suggestionStage === "review"
              ? "Choose what belongs. Nothing is created until you continue."
              : suggestionStage === "updating"
                ? "Your current lineup stays in place while Loomarr refreshes its suggestions."
                : suggestionStage === "generating"
                  ? "Loomarr is finding titles that match your brief."
                  : suggestionStage === "failed"
                    ? "Your brief is saved. Choose a recovery option below."
                    : "Describe what you want to watch. You'll choose the final titles before anything is created."
            : "Every channel Loomarr manages, and what's on right now."
        }
        actions={
          /* THE origination door (§12) — for both roles, since `/suggest` folded in here and
            this is now the only one in the app. Toggles the ChannelSuggestPanel open below:
            the create path IS describe→approve, inlined.

            The LABEL carries the role difference, not the presence: an admin adds a channel
            (their approval is the last step), a member requests one (an admin approves it).
            Same panel, same call — §7 enforces the gate server-side either way.

            ⚠ HIDDEN ON AN EMPTY GUIDE, because the empty state below already offers this
            exact action — same onClick, same panel. Two buttons a few hundred pixels apart
            firing one handler read as two different features, which is precisely how it
            landed for the maintainer on a fresh install.

            ⚠ …but NOT while the panel is open. Once `adding` is true this button is the
            "Close", and an empty guide is exactly when the operator is most likely to have
            opened it: hiding it there would leave the panel with no way out. So the door
            reappears the moment it becomes the exit. */
          (!isEmpty || adding) && (
            <Button
              variant={adding ? "outline" : "suggest"}
              onClick={() => (adding ? closePanel() : setAdding(true))}
              aria-expanded={adding}
            >
              {adding ? <X aria-hidden /> : <Sparkles aria-hidden />}
              {adding ? "Close" : isAdmin ? "Add a channel" : "Request a channel"}
            </Button>
          )
        }
      />

      {/* The inline create surface — describe a channel, review, approve, land on it. */}
      {adding && (
        <div className="border-border border-b bg-card px-7 py-4.5">
          <ChannelSuggestPanel
            initialIntent={initialIntent}
            initialJobId={initialJobId}
            onCreated={onCreated}
            onStartFresh={startFresh}
            onStageChange={setSuggestionStage}
          />
        </div>
      )}

      {/* Row 2 — the everyday guide questions stay visible: which day, where now is, and how
          much time is on screen. Precise planning controls live in the secondary View row. */}
      {!adding && channels.length > 0 && (
        <>
          <div className="flex flex-wrap items-center gap-3.5 border-border/60 border-b px-7 py-2.5">
            <div className="flex items-center gap-1.75">
              <Select value={String(dayOffset)} onValueChange={(v) => setDayOffset(Number(v))}>
                <SelectTrigger className="h-6.5 font-mono text-xs" aria-label="Day">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {dayChoices.map((d) => (
                    <SelectItem key={d} value={String(d)} className="font-mono text-xs">
                      {dayLabel(d, mountedAt)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              {/* A past day is RECOMPUTED from the current lineup, not a recording — and a
                  future one is a plan. The chip says which, and doubles as the way back. */}
              {dayOffset !== 0 && (
                <button
                  type="button"
                  onClick={() => setDayOffset(0)}
                  className={cn(
                    "cursor-pointer rounded-md border border-border px-2.5 py-0.5 font-mono text-2xs transition-colors hover:border-static-400",
                    dayOffset < 0 ? "text-static-400" : "text-tune",
                  )}
                >
                  {dayOffset < 0 ? "AS AIRED" : "SCHEDULED"} · back to today
                </button>
              )}
            </div>

            {/* Hour stepper. NOW is the anchor an operator returns to; the arrows nudge the
                window without changing the day. */}
            <div className="flex items-center gap-1">
              <Button
                variant="outline"
                size="icon"
                className="size-6.5"
                aria-label="Back an hour"
                onClick={() => setHourShift((h) => h - 1)}
              >
                ‹
              </Button>
              <Button
                variant="outline"
                size="sm"
                className={cn("h-6.5 font-mono text-2xs", atNow ? "text-static-400" : "text-signal")}
                onClick={() => {
                  setHourShift(0);
                  setDayOffset(0);
                  setStartHour(null);
                }}
              >
                NOW
              </Button>
              <Button
                variant="outline"
                size="icon"
                className="size-6.5"
                aria-label="Forward an hour"
                onClick={() => setHourShift((h) => h + 1)}
              >
                ›
              </Button>
            </div>

            {/* Span stays in the primary row: it changes the question the guide answers, and
                four compact choices are faster to scan than a select. */}
            <div className="flex items-center gap-1.75 sm:ml-auto">
              <Caption shout className="tracking-[0.06em]">
                Span
              </Caption>
              <div className="flex gap-1">
                {WINDOW_CHOICES.map((w) => {
                  const active = windowMinutes === Number(w.value);
                  return (
                    <button
                      key={w.value}
                      type="button"
                      onClick={() => setWindowMinutes(Number(w.value))}
                      aria-label={`Show ${w.description}`}
                      aria-pressed={active}
                      className={cn(
                        "min-w-8 cursor-pointer rounded-md border px-2 py-1 font-mono text-xs transition-colors",
                        active
                          ? "border-signal bg-signal-tint-15 text-signal"
                          : "border-border text-static-400 hover:border-static-400",
                      )}
                    >
                      {w.label}
                    </button>
                  );
                })}
              </div>
            </div>

            <Button
              variant="outline"
              size="sm"
              className={cn("h-6.5", (viewOptionsOpen || hasCustomView) && "text-signal")}
              aria-label={hasCustomView ? "View options, custom" : "View options"}
              aria-expanded={viewOptionsOpen}
              aria-controls="guide-view-options"
              onClick={() => setViewOptionsOpen((open) => !open)}
            >
              {hasCustomView ? (
                <span className="size-2 rounded-full bg-signal" aria-hidden />
              ) : (
                <SlidersHorizontal className="size-3.5" aria-hidden />
              )}
              View
            </Button>
          </div>

          {viewOptionsOpen && (
            <div
              id="guide-view-options"
              className="flex flex-wrap items-center justify-start gap-x-6 gap-y-2 border-border/60 border-b bg-card/40 px-7 py-2.5 sm:justify-end"
            >
              {/* Pin the window to a specific part of the chosen day without stepping there
                  one hour at a time. */}
              <div className="flex items-center gap-1.75">
                <Caption shout className="tracking-[0.06em]">
                  Start at
                </Caption>
                <Select
                  value={startHour === null ? "auto" : String(startHour)}
                  onValueChange={(v) => setStartHour(v === "auto" ? null : Number(v))}
                >
                  <SelectTrigger className="h-6.5 font-mono text-xs" aria-label="Start hour">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="auto" className="font-mono text-xs">
                      Now
                    </SelectItem>
                    {START_HOURS.map((h) => (
                      <SelectItem key={h} value={String(h)} className="font-mono text-xs">
                        {hourLabel(h)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}
        </>
      )}

      {!adding &&
        (isEmpty ? (
          // No channels yet is a real, expected state on a fresh install — not an error. The
          // one next action (§6) opens the same inline panel the header toggles, worded for
          // who is asking.
          //
          // ⚠ The LABEL matches the header's exactly. It used to read "Describe your first
          // channel" while the header said "Add a channel" and the panel that opened was
          // titled "Add a channel" — three names for one handler. One action, one name.
          // ⚠ The MOTION lives here, behind everything, not on the test card itself.
          //
          // A scanline sweeping over the bars was tried first and removed: to register on a
          // small element it had to be scaled up and brightened, at which point it dominated a
          // surface whose whole job is to stay calm and point at one button. Tuning it was the
          // wrong move; the premise was. A sweep is an attention-grabber, and "Dead air" is a
          // RESTING state.
          //
          // Ambient snow is the honest read: a real test card is not still because nothing is
          // happening, it is standing by. TvStatic is the design's own idle-surface layer (§1,
          // and its doc names empty states specifically), it is `motion-safe:` gated, and the
          // visual suite pins `reducedMotion: reduce` so it never rasterizes into a baseline.
          // The bars stay crisp and STILL on top, so the anchor never moves.
          <div className="relative flex flex-1 items-center justify-center overflow-hidden p-10">
            {/* ⚠ NO extra dimming. TvStatic is ALREADY faint by construction — its snow layer
              carries `opacity-[0.11]` internally — so an `opacity-60` wrapper (which this had)
              multiplied out to roughly 6.6% over a near-black background, i.e. nothing you
              could see. The login and wizard shells both use it plain, and this matches them. */}
            <TvStatic className="z-0" />
            <EmptyState
              className="relative z-10"
              // SMPTE bars over "Dead air": a test card is literally what a set showed when
              // nothing was broadcasting, so the motif IS the state rather than decoration
              // applied to it. Reuses the shell's ColorBars (aria-hidden, purely decorative)
              // rather than a new asset — it is the design's namesake strip, in the same
              // tokens the rest of the UI uses for state.
              //
              // The scanline drifts down it on a slow loop: a sign of life on the one screen
              // whose message is that nothing is on. `overflow-hidden` clips the sweep to the
              // strip, and `motion-safe:` means a reduced-motion visitor simply gets the bars.
              // The whole thing is aria-hidden, so none of it reaches assistive tech.
              // ⚠ STILL, deliberately — the motion is the ambient layer behind this, never the
              // card. A test card that jitters reads as a fault; one that holds while snow
              // drifts behind it reads as standing by, which is what "Dead air" means.
              //
              // A test-card BLOCK rather than the sidebar's thin 200×14 strip: presence comes
              // from size here, not movement. `h-16` on the wrapper with the bars told to fill
              // it — ColorBars fixes its own height, and `cn` puts className last so the
              // override wins.
              icon={
                <div className="h-16 w-50 overflow-hidden rounded-sm" aria-hidden>
                  <ColorBars size="lg" breathe className="h-full w-full" />
                </div>
              }
              title="Dead air"
              description="No channels yet. Describe the channel you want and Loomarr builds the lineup."
              action={{
                label: isAdmin ? "Add a channel" : "Request a channel",
                onClick: () => setAdding(true),
              }}
            />
          </div>
        ) : (
          // The pointer guide's programme card sits beside the grid, in its AdaptiveSplit.
          <div className="relative flex min-h-0 flex-1">
            {/* The next window is still loading and the grid below is the PREVIOUS one. An
                overlaid hairline, not a toolbar addition, so the chrome never shifts. */}
            {guide.isPlaceholderData && (
              <div
                role="status"
                aria-label="Loading guide"
                className="pointer-events-none absolute inset-x-0 top-0 z-30 h-0.5 animate-pulse bg-signal"
              />
            )}
            {layout && (
              <AdaptiveSplit
                breakpoint={SPLIT_BREAKPOINT}
                flex={1}
                minHeight={0}
                primary={
                  <GuideGrid
                    layout={layout}
                    nowMs={nowMs}
                    onHover={setHovered}
                    onMove={onMove}
                    onOpenChannel={onOpenChannel}
                    onSelect={setSelection}
                    renderRowMenu={renderRowMenu}
                    selection={selected}
                  />
                }
                secondary={
                  <GuideProgrammeDetail
                    layout={layout}
                    renderArtwork={renderArtwork}
                    selection={hovered ?? selected}
                  />
                }
                secondaryWidth={360}
              />
            )}
          </div>
        ))}
    </div>
  );
};

export { GuidePage };
