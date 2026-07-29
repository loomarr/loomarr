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
  FillerIngestEvent,
  JobEvent,
  LlmPullEvent,
  PlayoutEvent,
  SuggestionEvent,
  TitleEvent,
} from "./events.type";

const EVENTS_URL = "/v1/events";

// openEventStream subscribes to the named SSE frames and returns a close fn. Same-
// origin with cookie auth (withCredentials). Malformed frames are ignored (latency
// bus, never load-bearing).
const openEventStream = (handlers: EventHandlers, url: string = EVENTS_URL): (() => void) => {
  const es = new EventSource(url, { withCredentials: true });
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

// useLoomarrEvents opens the stream for the app's lifetime and wires the standard
// invalidations; `extra` handlers (e.g. a workspace consuming suggestion phases)
// run after invalidation and are read through a ref so updating them never
// reconnects the stream.
const useLoomarrEvents = (extra?: EventHandlers): void => {
  const qc = useQueryClient();
  const extraRef = useRef(extra);
  extraRef.current = extra;

  useEffect(() => {
    return openEventStream({
      onTitle: (e) => {
        invalidateByPrefix(qc, "/v1/titles");
        invalidateByPrefix(qc, "/v1/suggestions");
        extraRef.current?.onTitle?.(e);
      },
      onChannel: (e) => {
        invalidateByPrefix(qc, "/v1/channels");
        extraRef.current?.onChannel?.(e);
      },
      onSuggestion: (e) => {
        invalidateByPrefix(qc, "/v1/suggestions");
        extraRef.current?.onSuggestion?.(e);
      },
      onLlmPull: (e) => {
        invalidateByPrefix(qc, "/v1/system/llm");
        extraRef.current?.onLlmPull?.(e);
      },
      onPlayout: (e) => {
        // A channel started or stopped encoding, so the telemetry endpoint's answer changed.
        // Invalidating is the whole job: the frame carries only a count, and
        // GET /v1/playout/sessions owns the shape the dashboard renders (§8 — SSE is the
        // latency path, the GET is truth).
        invalidateByPrefix(qc, "/v1/playout/sessions");
        extraRef.current?.onPlayout?.(e);
      },
      onFillerIngest: (e) => {
        // Downloaded files are not clips until Tunarr scans the folder, so a finished
        // ingest does NOT invalidate the catalog on its own — the operator runs Sync,
        // which is what actually changes /v1/filler. Invalidating here would refetch an
        // unchanged list and imply the clips had arrived.
        extraRef.current?.onFillerIngest?.(e);
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
        // This is why the feed does NOT poll, while the Services panel beside it does: a
        // feed row is a real event the server knows at write time, whereas a probe result
        // only exists because the server went and asked.
        invalidateByPrefix(qc, "/v1/activity");
        extraRef.current?.onActivity?.(e);
      },
    });
  }, [qc]);
};

export { EVENTS_URL, invalidateByPrefix, openEventStream, useLoomarrEvents };
