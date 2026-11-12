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
  EventStreamMessage,
  EventType,
  FrameHandler,
} from "./events.type";

const EVENTS_URL = "/v1/events";

// How long the browser stream waits before opening a new EventSource in place of a closed one.
const STREAM_REOPEN_MS = 3000;

// EventSource.CLOSED, by value: a stand-in source in a test may not carry the constant, and an
// undefined one would make every error look like a source closed for good.
const SOURCE_CLOSED = 2;

// The browser retries a dropped connection by itself, but a refused response closes an
// EventSource for good: a proxy's 502 while the server restarts for an update is enough. This
// stream opens a new one in its place, with the same listeners, so it outlives a restart. Each
// source that connects fires "open", which is how openFrameStream hears each open.
const browserEventStreamFactory: EventStreamFactory = (url) => {
  const listeners: [string, (event: EventStreamMessage) => void][] = [];
  let source: EventSource;
  let closed = false;
  let reopen: ReturnType<typeof setTimeout> | undefined;
  const attach = (type: string, listener: (event: EventStreamMessage) => void) =>
    source.addEventListener(type, (event) => listener(event as MessageEvent<string>));
  const connect = () => {
    source = new EventSource(url, { withCredentials: true });
    for (const [type, listener] of listeners) attach(type, listener);
    const current = source;
    current.addEventListener("error", () => {
      if (closed || current.readyState !== SOURCE_CLOSED) return;
      reopen = setTimeout(connect, STREAM_REOPEN_MS);
    });
  };
  connect();
  return {
    addEventListener: (type, listener) => {
      listeners.push([type, listener]);
      attach(type, listener);
    },
    close: () => {
      closed = true;
      clearTimeout(reopen);
      source.close();
    },
  };
};

// What a frame invalidates: every query whose first key element (the URL, per orval's fetch
// client) starts with one of `prefixes`, optionally only when `when` accepts the payload.
//
// `fed` names the reads this frame alone keeps current: a query whose URL matches one is trusted
// (staleTime Infinity) and refetched only when its frame, a reconnect, or its own mutation
// invalidates it. It is an allowlist of exact URLs: a read that also moves with the clock (now and
// next, a timeline) or carries a signed URL never belongs here, and stays stale-while-revalidate.
interface Invalidation<T extends EventType> {
  prefixes: readonly string[];
  when?: (payload: EventPayloads[T]) => boolean;
  fed?: readonly RegExp[];
}

// A collection URL and its one-segment detail ("/v1/titles", "/v1/titles/{key}"), nothing deeper.
// `except` names sibling segments that are a different read ("/v1/channels/now-next").
const listAndDetail = (url: string, except: readonly string[] = []): RegExp => {
  const notExcepted = except.length > 0 ? `(?!(?:${except.join("|")})$)` : "";
  return new RegExp(`^${url}(?:/${notExcepted}[^/]+)?$`);
};

// Invalidating is the whole job: a frame carries a count or a few ids, and the GETs own the
// shapes the screens render (§8 — SSE is the latency path, the GET is truth). A frame with no
// prefixes is a pure hand-off to listeners.
const EVENT_INVALIDATIONS: { [T in EventType]: Invalidation<T> } = {
  title: {
    prefixes: ["/v1/titles", "/v1/proposals"],
    fed: [listAndDetail("/v1/titles"), listAndDetail("/v1/proposals")],
  },
  // `/v1/channels` covers the list, now/next, and each channel's detail, pods, upcoming and
  // cycle reads; `/v1/guide` covers every cached guide window, prefetched neighbours included.
  // Only the list and a channel's detail are fed: now/next, the timeline and upcoming also move
  // as the clock passes a programme's end, and a guide window is a view over that same clock.
  channel: { prefixes: ["/v1/channels", "/v1/guide"], fed: [listAndDetail("/v1/channels", ["now-next"])] },
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
  job: { prefixes: ["/v1/jobs"], fed: [listAndDetail("/v1/jobs")] },
  // The migration page reads progress from the frame itself and refetches its status through
  // its own listener, so the bus has nothing to invalidate.
  database: { prefixes: [] },
  // A Dashboard feed row was written (§12, V32). The frame is empty on purpose — refetch the
  // list rather than appending, because this bus drops frames for a slow subscriber and a
  // locally-assembled list would be missing rows.
  activity: { prefixes: ["/v1/activity"], fed: [listAndDetail("/v1/activity")] },
  health: { prefixes: ["/v1/diagnostics/health"], fed: [/^\/v1\/diagnostics\/health$/] },
  // A committed settings write, from any tab or writer. Every read a setting can change
  // refetches here, so no save path lists queries itself: `/v1/setup` is the checklist and wizard
  // state, `/v1/system/llm` the AI provider status, catalog and pull progress, and
  // `/v1/filler/readiness` the storage summary derived from the filler allowances.
  settings: {
    prefixes: ["/v1/settings", "/v1/setup", "/v1/system/llm", "/v1/filler/readiness"],
    fed: [listAndDetail("/v1/settings"), /^\/v1\/setup$/],
  },
  // How far a library import is, whole and latest-wins: the screens showing it write each frame
  // into their status read (applyLibraryScanFrame) rather than refetch it a second. Once
  // everything is idle the status refetches once, for the last scan's time and the counts.
  library_scan: {
    prefixes: ["/v1/library/status"],
    when: (e) => e.step === "idle",
    fed: [/^\/v1\/library\/status$/],
  },
  // The Library's titles changed (written, ready, or gone). The frame is empty on purpose:
  // the grid, a title's page, Ready so far and Needs you refetch, because a list assembled from
  // frames would lose rows this bus drops.
  library_titles: {
    prefixes: ["/v1/library/items", "/v1/library/needs"],
    fed: [listAndDetail("/v1/library/items"), /^\/v1\/library\/needs$/],
  },
  // A media folder stopped or started opening, or emptied or filled (a share unmounted or mounted
  // again), found by a scan pass rather than by anything done on screen: the folders' cards and
  // the status read again, so a warning appears and clears without a reload.
  library_roots: { prefixes: ["/v1/library/roots", "/v1/library/status"] },
};

// isFrameFed: does an SSE frame keep this query current? The client gives such a read
// staleTime Infinity, so it never refetches on mount or focus; its frame, a reconnect or a
// mutation does. Keys are orval's `[url, params?]`; anything else is not fed.
const isFrameFed = (queryKey: readonly unknown[]): boolean => {
  const url = queryKey[0];
  if (typeof url !== "string") return false;
  return Object.values(EVENT_INVALIDATIONS).some((row) => row.fed?.some((pattern) => pattern.test(url)));
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
//
// `onOpen` runs each time the stream opens, the first time included. Frames sent before it opened
// were never heard: those of a drop, and those after a read the page made before subscribing (a
// route loader reads before the layout opens the stream). So the caller reads the truth again
// rather than trusting what it shows.
const openFrameStream = (
  onFrame: FrameHandler,
  url: string = EVENTS_URL,
  createStream: EventStreamFactory = browserEventStreamFactory,
  onOpen?: () => void,
): (() => void) => {
  const es = createStream(url);
  es.addEventListener("open", () => onOpen?.());
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
const invalidateByPrefix = (qc: QueryClient, prefix: string): Promise<void> =>
  qc.invalidateQueries({
    predicate: (q) => typeof q.queryKey[0] === "string" && (q.queryKey[0] as string).startsWith(prefix),
  });

// invalidateForFrame applies a frame's row of the invalidation table. The promise settles when
// the active queries have refetched, for a caller that must not show stale data in between.
const invalidateForFrame = async <T extends EventType>(
  qc: QueryClient,
  type: T,
  payload: EventPayloads[T],
): Promise<void> => {
  const { prefixes, when }: Invalidation<T> = EVENT_INVALIDATIONS[type];
  if (when && !when(payload)) return;
  await Promise.all(prefixes.map((prefix) => invalidateByPrefix(qc, prefix)));
};

// invalidateSettings is the settings row for a save path that just wrote a setting: the writer's
// own tab refetches at once (no wait for the frame), through the same mapping other tabs get.
// Awaiting it before clearing staged edits is what keeps a saved field from flashing its old value.
const invalidateSettings = (qc: QueryClient): Promise<void> => invalidateForFrame(qc, "settings", {});

// invalidateMissedFrames refetches everything a frame could have changed: every prefix in the
// table, whatever its `when`, because which frames went unheard before the stream opened is not
// known. A scan that started or finished in the gap is why a frozen bar or a stale "All caught
// up" cannot outlast the open. Only the reads on screen refetch; the rest wait for their next use.
const invalidateMissedFrames = async (qc: QueryClient): Promise<void> => {
  const prefixes = new Set(EVENT_TYPES.flatMap((type) => EVENT_INVALIDATIONS[type].prefixes));
  await Promise.all([...prefixes].map((prefix) => invalidateByPrefix(qc, prefix)));
};

// useLoomarrEvents opens the stream for the app's lifetime and applies the invalidation table
// to every frame; `onFrame` (e.g. the provider fanning payloads out to screens) runs after the
// invalidation and is read through a ref so updating it never reconnects the stream. Each time the
// stream opens, first or after a drop, every read a frame feeds refetches.
const useLoomarrEvents = (onFrame?: FrameHandler): void => {
  const qc = useQueryClient();
  const onFrameRef = useRef(onFrame);
  onFrameRef.current = onFrame;

  useEffect(() => {
    return openFrameStream(
      (type, payload) => {
        void invalidateForFrame(qc, type, payload);
        onFrameRef.current?.(type, payload);
      },
      undefined,
      undefined,
      () => void invalidateMissedFrames(qc),
    );
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
  invalidateMissedFrames,
  invalidateSettings,
  isFrameFed,
  listAndDetail,
  openEventStream,
  openFrameStream,
  useLoomarrEvents,
};
