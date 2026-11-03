// The SSE invalidation bus (frontend-design §4.2; main doc §8). It mirrors the
// BE's /v1/events frame vocabulary and turns each frame into TanStack Query cache
// invalidations — the coarse "invalidate-and-refetch" contract, never a surgical
// patch stream: a dropped frame is a latency bug, GET is the source of truth on
// reconnect (§8, finding 5 of the FE↔BE audit). Platform-agnostic: the parsing +
// invalidation logic is shared; only the EventSource construction is web-specific
// (RN injects a polyfill with the same interface).
//
// ONE list of frame types, ONE dispatcher. `EVENT_INVALIDATIONS` is keyed by every event name
// the generated spec declares, so a new spec event is a compile error here until it is given an
// invalidation entry; the stream subscribes by looping that table's keys and hands every frame
// to a single `onFrame(type, payload)`. No per-frame handler list is written by hand anywhere.

import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import type {
  EventHandlerName,
  EventHandlers,
  EventPayloads,
  EventStreamFactory,
  EventType,
  FrameHandler,
} from "./events.type";

const EVENTS_URL = "/v1/events";

const browserEventStreamFactory: EventStreamFactory = (url) => {
  const source = new EventSource(url, { withCredentials: true });
  return {
    addEventListener: (type, listener) =>
      source.addEventListener(type, (event) => listener(event as MessageEvent<string>)),
    close: () => source.close(),
  };
};

// What a frame invalidates: every query whose first key element (the URL, per orval's fetch
// client) starts with one of `prefixes`, optionally only when `when` accepts the payload.
interface Invalidation<T extends EventType> {
  prefixes: readonly string[];
  when?: (payload: EventPayloads[T]) => boolean;
}

// Invalidating is the whole job: a frame carries a count or a few ids, and the GETs own the
// shapes the screens render (§8 — SSE is the latency path, the GET is truth). A frame with no
// prefixes is a pure hand-off to listeners.
const EVENT_INVALIDATIONS: { [T in EventType]: Invalidation<T> } = {
  title: { prefixes: ["/v1/titles", "/v1/proposals"] },
  // `/v1/channels` covers the list, now/next, and each channel's detail, pods, upcoming and
  // cycle reads; `/v1/guide` covers every cached guide window, prefetched neighbours included.
  channel: { prefixes: ["/v1/channels", "/v1/guide"] },
  // A frame also fires as a running request's stage or picks change, and the journey read
  // (GET /v1/proposal-jobs) carries that live progress — so the Requests card and detail catch
  // up on the frame instead of waiting for their poll.
  suggestion: { prefixes: ["/v1/proposals", "/v1/proposal-jobs"] },
  llm_pull: { prefixes: ["/v1/system/llm"] },
  // A channel started or stopped encoding, so both the telemetry endpoint AND the doctor's
  // answer changed (encoder, cold-start time, GPU contention); the doctor rides the same
  // start/stop cadence as sessions.
  playout: { prefixes: ["/v1/playout/sessions", "/v1/playout/status"] },
  // Since §10 V56 a success is published only AFTER the ordinary catalog sync and one durable
  // pipeline nudge, so incoming, watch status and the catalog are already authoritative then.
  filler_ingest: { prefixes: ["/v1/filler"], when: (e) => e.status === "success" },
  // A split PROPOSAL is not a catalog change: segments become clips only when the operator
  // confirms, and that confirm's own mutation invalidates /v1/filler. The frame's job is the
  // handoff — the terminal success carries the proposal id the review route navigates to.
  filler_split: { prefixes: [] },
  // One clip moved a rung (§10 V51b). ⚠ Only a TERMINAL frame invalidates: forty clips × ten
  // rungs is 400 frames, and invalidating on each would refetch the incoming queue 400 times to
  // render a queue that already draws itself from the frame. A clip reaching `filed` changes the
  // CATALOG, and nothing mounted may be listening, so the bus invalidates rather than a panel.
  filler_clip: { prefixes: ["/v1/filler"], when: (e) => e.disposition !== "running" },
  // A scheduled job changed state (§18.1); BE is the single timing source.
  job: { prefixes: ["/v1/jobs"] },
  // The migration page reads progress from the frame itself and refetches its status through
  // its own listener, so the bus has nothing to invalidate.
  database: { prefixes: [] },
  // A Dashboard feed row was written (§12, V32). The frame is empty on purpose — refetch the
  // list rather than appending, because this bus drops frames for a slow subscriber and a
  // locally-assembled list would be missing rows.
  activity: { prefixes: ["/v1/activity"] },
  health: { prefixes: ["/v1/diagnostics/health"] },
};

// The frame types the stream subscribes to: every event the spec declares.
const EVENT_TYPES = Object.keys(EVENT_INVALIDATIONS) as EventType[];

// "filler_clip" -> "onFillerClip": the handler name a consumer writes for a frame type.
const eventHandlerName = <T extends EventType>(type: T): EventHandlerName<T> =>
  `on${type
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join("")}` as EventHandlerName<T>;

// dispatchFrame calls the handler a consumer registered for this frame's type, if any.
const dispatchFrame = <T extends EventType>(
  handlers: EventHandlers,
  type: T,
  payload: EventPayloads[T],
): void => {
  const handler = handlers[eventHandlerName(type)] as ((e: EventPayloads[T]) => void) | undefined;
  handler?.(payload);
};

// openFrameStream subscribes to every frame type and hands each parsed frame to the one
// `onFrame`. Same-origin with cookie auth (withCredentials). Malformed frames are ignored
// (latency bus, never load-bearing). Returns a close fn.
const openFrameStream = (
  onFrame: FrameHandler,
  url: string = EVENTS_URL,
  createStream: EventStreamFactory = browserEventStreamFactory,
): (() => void) => {
  const es = createStream(url);
  for (const type of EVENT_TYPES) {
    es.addEventListener(type, (ev) => {
      try {
        onFrame(type, JSON.parse(ev.data) as EventPayloads[typeof type]);
      } catch {
        /* ignore malformed frame */
      }
    });
  }
  return () => es.close();
};

// openEventStream is openFrameStream for a consumer that wants named handlers (the native
// client's channel refresh), not the raw dispatcher.
const openEventStream = (
  handlers: EventHandlers,
  url?: string,
  createStream?: EventStreamFactory,
): (() => void) =>
  openFrameStream((type, payload) => dispatchFrame(handlers, type, payload), url, createStream);

// invalidateByPrefix invalidates every query whose first key element (the URL, per
// orval's fetch client) starts with prefix — coarse but correct.
const invalidateByPrefix = (qc: QueryClient, prefix: string): void => {
  void qc.invalidateQueries({
    predicate: (q) => typeof q.queryKey[0] === "string" && (q.queryKey[0] as string).startsWith(prefix),
  });
};

// invalidateForFrame applies a frame's row of the invalidation table.
const invalidateForFrame = <T extends EventType>(
  qc: QueryClient,
  type: T,
  payload: EventPayloads[T],
): void => {
  const { prefixes, when }: Invalidation<T> = EVENT_INVALIDATIONS[type];
  if (when && !when(payload)) return;
  for (const prefix of prefixes) invalidateByPrefix(qc, prefix);
};

// useLoomarrEvents opens the stream for the app's lifetime and applies the invalidation table
// to every frame; `onFrame` (e.g. the provider fanning payloads out to screens) runs after the
// invalidation and is read through a ref so updating it never reconnects the stream.
const useLoomarrEvents = (onFrame?: FrameHandler): void => {
  const qc = useQueryClient();
  const onFrameRef = useRef(onFrame);
  onFrameRef.current = onFrame;

  useEffect(() => {
    return openFrameStream((type, payload) => {
      invalidateForFrame(qc, type, payload);
      onFrameRef.current?.(type, payload);
    });
  }, [qc]);
};

export {
  browserEventStreamFactory,
  dispatchFrame,
  EVENT_INVALIDATIONS,
  EVENT_TYPES,
  EVENTS_URL,
  eventHandlerName,
  invalidateByPrefix,
  invalidateForFrame,
  openEventStream,
  openFrameStream,
  useLoomarrEvents,
};
