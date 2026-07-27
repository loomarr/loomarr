import { channelsApi, type GuideAiring } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Sparkles, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useAuth } from "@/auth";
import { EmptyState, ErrorState, GuideDetailCard, GuideGrid } from "@/components/loomarr";
import {
  Badge,
  Button,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui";
import { useLoomarrEventListener } from "@/events";
import { ChannelSuggestPanel } from "@/suggest";
import { ChannelRowMenu } from "../channel-row-menu";
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

// Zoom scales the CHROME (rail width, row height, type) on the mock's own stops — never the
// time scale. The window always fits; you change how much detail each row carries.
const ZOOM_STOPS = [0.75, 0.875, 1, 1.15, 1.35, 1.6] as const;
const DEFAULT_ZOOM_INDEX = 2;

// How far a window may span. The mock's 2H/4H/6H, plus a whole day for planning.
const WINDOW_CHOICES = [
  { value: "120", label: "2 hours" },
  { value: "240", label: "4 hours" },
  { value: "360", label: "6 hours" },
  { value: "720", label: "12 hours" },
] as const;
const DEFAULT_WINDOW_MINUTES = 240;

// How much of the window sits behind "now" on TODAY. A guide opens on the present, but a
// programme already in progress needs its real start on screen or it appears to begin the
// moment the page was opened.
const LOOKBACK_MINUTES = 30;

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

const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

// The day picker's label: relative for the days people actually mean, absolute beyond that.
const dayLabel = (offset: number, now: number): string => {
  if (offset === 0) return "Today";
  if (offset === 1) return "Tomorrow";
  if (offset === -1) return "Yesterday";
  const d = new Date(startOfDay(new Date(now)) + offset * 86_400_000);
  return d.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" });
};

const GuidePage = ({ initialIntent }: GuidePageProps) => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { isAdmin } = useAuth();
  const [zoomIndex, setZoomIndex] = useState<number>(DEFAULT_ZOOM_INDEX);
  const [nowMs, setNowMs] = useState<number>(() => Date.now());
  const [inspected, setInspected] = useState<GuideAiring | null>(null);
  const [dayOffset, setDayOffset] = useState<number>(0);
  const [windowMinutes, setWindowMinutes] = useState<number>(DEFAULT_WINDOW_MINUTES);
  // The inline "describe a channel" surface. Creating a channel IS describing one (Suggest),
  // so the create path is the ChannelSuggestPanel expanded in place — no separate empty-shell
  // dialog. `adding` toggles it open.
  //
  // Opens on arrival when the wizard handed off a template (§13), so the operator lands on a
  // filled form rather than a bare grid wondering where their pick went. Lazy initializer:
  // read once at mount, so closing it stays closed.
  const [adding, setAdding] = useState(() => Boolean(initialIntent));

  // Closing also CLEARS `?intent=`. Leaving it would make a refresh silently re-open the
  // panel with a template the operator already dismissed — and right after the wizard, on a
  // first-run install, a refresh is likely rather than exotic. `replace: true` keeps the
  // dismissal out of history: closing a panel is not a place you navigate back to.
  const closePanel = () => {
    setAdding(false);
    if (initialIntent) void navigate({ to: "/guide", search: {}, replace: true });
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

  // Today opens on the present (minus a little lookback); any other day starts at midnight,
  // because "what is on at 3pm on Thursday" is the question a future day is asked.
  const { from, to } = useMemo(() => {
    const span = windowMinutes * 60_000;
    if (dayOffset === 0) {
      const start = mountedAt - LOOKBACK_MINUTES * 60_000;
      return { from: start, to: start + span };
    }
    const start = startOfDay(new Date(mountedAt)) + dayOffset * 86_400_000;
    return { from: start, to: start + span };
  }, [dayOffset, windowMinutes, mountedAt]);

  const guide = channelsApi.useChannelGuide({ from, to }, { query: { retry: false } });

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

  const body = guide.data?.status === 200 ? guide.data.data : undefined;
  const channels = useMemo(() => body?.channels ?? [], [body]);

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
    <div className="flex min-h-0 flex-1 flex-col gap-4 p-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="font-semibold text-xl">Channels</h1>
          <p className="text-muted-foreground text-sm">
            Every channel Loomarr manages, and what's on right now.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {/* THE origination door (§12) — for both roles, since `/suggest` folded in here and
              this is now the only one in the app. Toggles the ChannelSuggestPanel open below:
              the create path IS describe→approve, inlined. It leads the control cluster
              because originating a channel outranks re-framing the window.

              The LABEL carries the role difference, not the presence: an admin adds a channel
              (their approval is the last step), a member requests one (an admin approves it).
              Same panel, same call — §7 enforces the gate server-side either way. */}
          <Button
            variant={adding ? "outline" : "suggest"}
            size="sm"
            onClick={() => (adding ? closePanel() : setAdding(true))}
            aria-expanded={adding}
          >
            {adding ? <X aria-hidden /> : <Sparkles aria-hidden />}
            {adding ? "Close" : isAdmin ? "Add a channel" : "Request a channel"}
          </Button>

          {/* A past day is RECOMPUTED from the current lineup, so it is labelled as such —
              "as aired" would promise a recording of history this cannot give. */}
          {dayOffset < 0 && <Badge variant="neutral">Recomputed</Badge>}
          {dayOffset > 0 && <Badge variant="neutral">Scheduled</Badge>}

          <Select value={String(dayOffset)} onValueChange={(v) => setDayOffset(Number(v))}>
            <SelectTrigger className="w-40" aria-label="Day">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {dayChoices.map((d) => (
                <SelectItem key={d} value={String(d)}>
                  {dayLabel(d, mountedAt)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select value={String(windowMinutes)} onValueChange={(v) => setWindowMinutes(Number(v))}>
            <SelectTrigger className="w-32" aria-label="Window span">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {WINDOW_CHOICES.map((w) => (
                <SelectItem key={w.value} value={w.value}>
                  {w.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* Jumping back to now is one action, not "work out which day today was". */}
          {dayOffset !== 0 && (
            <Button variant="outline" size="sm" onClick={() => setDayOffset(0)}>
              Now
            </Button>
          )}

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
      </div>

      {/* The inline create surface — describe a channel, review, approve, land on it. */}
      {adding && <ChannelSuggestPanel initialIntent={initialIntent} onCreated={onCreated} />}

      {channels.length === 0 && !guide.isLoading ? (
        // No channels yet is a real, expected state on a fresh install — not an error. The
        // one next action (§6) opens the same inline panel the header toggles, worded for
        // who is asking.
        <EmptyState
          title="Dead air"
          description="No channels yet. Describe the channel you want and Loomarr builds the lineup."
          action={{
            label: isAdmin ? "Describe your first channel" : "Request your first channel",
            onClick: () => setAdding(true),
          }}
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
            // The now-line belongs to TODAY. Drawing it on another day would mark an instant
            // that is not in the window being shown.
            nowMs={dayOffset === 0 ? nowMs : undefined}
            onInspect={setInspected}
            onSelectChannel={(id) => navigate({ to: "/channels/$id", params: { id } })}
            renderRowMenu={(ch) => (
              <ChannelRowMenu channel={{ id: ch.channelId, name: ch.name, status: ch.status }} />
            )}
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
