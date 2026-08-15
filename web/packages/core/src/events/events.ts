// The SSE invalidation bus (frontend-design §4.2; main doc §8). It mirrors the
// BE's /v1/events frame vocabulary and turns each frame into TanStack Query cache
// invalidations — the coarse "invalidate-and-refetch" contract, never a surgical
// patch stream: a dropped frame is a latency bug, GET is the source of truth on
// reconnect (§8, finding 5 of the FE↔BE audit). Platform-agnostic: the parsing +
// invalidation logic is shared; only the EventSource construction is web-specific
// (RN injects a polyfill with the same interface).

import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import type {
  ActivityEvent,
  ChannelEvent,
  DatabaseEvent,
  EventHandlers,
  FillerClipEvent,
  FillerIngestEvent,
  FillerSplitEvent,
  JobEvent,
  LlmPullEvent,
  PlayoutEvent,
  SuggestionEvent,
  TitleEvent,
} from "./events.type";

const EVENTS_URL = "/v1/events";

interface EventStreamLifecycle {
  onOpen?: () => void;
}

// openEventStream subscribes to the named SSE frames and returns a close fn. Same-
// origin with cookie auth (withCredentials). Malformed frames are ignored (latency
// bus, never load-bearing).
const openEventStream = (
  handlers: EventHandlers,
  url: string = EVENTS_URL,
  lifecycle: EventStreamLifecycle = {},
): (() => void) => {
  const es = new EventSource(url, { withCredentials: true });

  // EventSource emits `open` for the initial connection and for every reconnect. The stream
  // cannot replay frames lost during a gap, so `open` is the point where consumers must ask the
  // GET sources of truth for a fresh snapshot.
  es.addEventListener("open", () => lifecycle.onOpen?.());
  const on = <T>(type: string, cb?: (e: T) => void) => {
    if (!cb) return;
    es.addEventListener(type, (ev) => {
      try {
        cb(JSON.parse((ev as MessageEvent).data) as T);
      } catch {
        /* ignore malformed frame */
      }
    });
  };
  on<TitleEvent>("title", handlers.onTitle);
  on<ChannelEvent>("channel", handlers.onChannel);
  on<SuggestionEvent>("suggestion", handlers.onSuggestion);
  on<LlmPullEvent>("llm_pull", handlers.onLlmPull);
  on<FillerIngestEvent>("filler_ingest", handlers.onFillerIngest);
  on<FillerSplitEvent>("filler_split", handlers.onFillerSplit);
  on<FillerClipEvent>("filler_clip", handlers.onFillerClip);
  on<JobEvent>("job", handlers.onJob);
  on<PlayoutEvent>("playout", handlers.onPlayout);
  on<DatabaseEvent>("database", handlers.onDatabase);
  on<ActivityEvent>("activity", handlers.onActivity);
  return () => es.close();
};

// invalidateByPrefix invalidates every query whose first key element (the URL, per
// orval's fetch client) starts with prefix — coarse but correct.
const invalidateByPrefix = (qc: QueryClient, prefix: string): void => {
  void qc.invalidateQueries({
    predicate: (q) => typeof q.queryKey[0] === "string" && (q.queryKey[0] as string).startsWith(prefix),
  });
};

const EVENT_BACKED_PREFIXES = [
  "/v1/dashboard/summary",
  "/v1/titles",
  "/v1/proposals",
  "/v1/channels",
  "/v1/playout/sessions",
  "/v1/playout/status",
  "/v1/system/llm",
  "/v1/filler",
  "/v1/jobs",
  "/v1/activity",
] as const;

const resyncEventBackedQueries = (qc: QueryClient): void => {
  for (const prefix of EVENT_BACKED_PREFIXES) invalidateByPrefix(qc, prefix);
};

// useLoomarrEvents opens the stream for the app's lifetime and wires the standard
// invalidations; `extra` handlers (e.g. a workspace consuming suggestion phases)
// run after invalidation and are read through a ref so updating them never
// reconnects the stream.
const useLoomarrEvents = (extra?: EventHandlers): void => {
  const qc = useQueryClient();
  const extraRef = useRef(extra);
  extraRef.current = extra;

  useEffect(() => {
    return openEventStream(
      {
        onTitle: (e) => {
          invalidateByPrefix(qc, "/v1/titles");
          invalidateByPrefix(qc, "/v1/proposals");
          invalidateByPrefix(qc, "/v1/dashboard/summary");
          extraRef.current?.onTitle?.(e);
        },
        onChannel: (e) => {
          invalidateByPrefix(qc, "/v1/channels");
          invalidateByPrefix(qc, "/v1/dashboard/summary");
          extraRef.current?.onChannel?.(e);
        },
        onSuggestion: (e) => {
          invalidateByPrefix(qc, "/v1/proposals");
          invalidateByPrefix(qc, "/v1/dashboard/summary");
          extraRef.current?.onSuggestion?.(e);
        },
        onLlmPull: (e) => {
          invalidateByPrefix(qc, "/v1/system/llm");
          extraRef.current?.onLlmPull?.(e);
        },
        onPlayout: (e) => {
          // A channel started or stopped encoding, so both the telemetry endpoint AND the doctor's
          // answer changed (encoder, cold-start time, GPU contention). Invalidating is the whole job:
          // the frame carries only a count, and the GETs own the shapes the dashboard renders (§8 —
          // SSE is the latency path, the GET is truth). The doctor is the same start/stop cadence as
          // sessions, so it rides the same frame rather than needing a stream of its own.
          invalidateByPrefix(qc, "/v1/playout/sessions");
          invalidateByPrefix(qc, "/v1/playout/status");
          extraRef.current?.onPlayout?.(e);
        },
        onFillerIngest: (e) => {
          // Downloaded files are not clips until Tunarr scans the folder, so a finished
          // ingest does NOT invalidate the catalog on its own — the operator runs Sync,
          // which is what actually changes /v1/filler. Invalidating here would refetch an
          // unchanged list and imply the clips had arrived.
          extraRef.current?.onFillerIngest?.(e);
        },
        onFillerSplit: (e) => {
          // A split PROPOSAL is not a catalog change: segments become clips only when the
          // operator confirms, and that confirm's own mutation invalidates /v1/filler. The
          // frame's job here is the handoff — the terminal success carries the proposal id
          // the review route navigates to — so it passes straight through to listeners.
          extraRef.current?.onFillerSplit?.(e);
        },
        onFillerClip: (e) => {
          // One clip moved a rung (§10 V51b). ⚠ **Only a TERMINAL frame invalidates**, and the
          // asymmetry is the whole point: forty clips × eight rungs is 320 frames, so invalidating
          // on each would refetch the incoming queue 320 times to render a queue that already knows
          // how to draw itself from the frame. A running frame is a repaint; leaving it is what the
          // consumer's cache merge is for.
          //
          // ⚠ The invalidation lives HERE rather than in the panel because it is not about the
          // panel. A clip reaching `filed` changes the CATALOG, and the operator watching the
          // catalog tab — or the dashboard, or nothing at all — has no pipeline listener mounted to
          // notice. The rule "the thing that changed is invalidated by whoever knows it changed"
          // is what keeps that from depending on which tab happens to be open.
          if (e.disposition !== "running") {
            invalidateByPrefix(qc, "/v1/filler");
          }
          extraRef.current?.onFillerClip?.(e);
        },
        onJob: (e) => {
          // A scheduled job changed state (§18.1) — refetch /v1/jobs so the Tasks page
          // renders the BE's fresh last/next-run + status. BE is the single timing source.
          invalidateByPrefix(qc, "/v1/jobs");
          extraRef.current?.onJob?.(e);
        },
        onActivity: (e) => {
          // A Dashboard feed row was written (§12, V32). The frame is empty on purpose —
          // refetch the list rather than appending from the payload, because this bus drops
          // frames for a slow subscriber and a locally-assembled list would be missing rows.
          //
          // The dashboard also keeps a slow fallback poll. This frame remains the low-latency path;
          // the poll and reconnect resync make a dropped frame harmless.
          invalidateByPrefix(qc, "/v1/activity");
          extraRef.current?.onActivity?.(e);
        },
      },
      EVENTS_URL,
      { onOpen: () => resyncEventBackedQueries(qc) },
    );
  }, [qc]);
};

export { EVENTS_URL, invalidateByPrefix, openEventStream, resyncEventBackedQueries, useLoomarrEvents };
