import { type EventHandlers, useLoomarrEvents } from "@loomarr/core";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef } from "react";
import type { LoomarrEventsProviderProps, Subscribe } from "./events-provider.type";

// ONE SSE stream, many listeners. `useLoomarrEvents` opens an EventSource, and the app
// layout already mounts it for cache invalidation — so a screen that also wants the
// frame *payload* (the Suggest workspace needs the searching/reasoning/scoring phases,
// which exist only on the wire) must not call the hook again and open a second stream.
// This provider owns the single subscription and fans frames out to whoever asked.
const EventsContext = createContext<Subscribe | undefined>(undefined);

const LoomarrEventsProvider = ({ children }: LoomarrEventsProviderProps) => {
  const listeners = useRef(new Set<EventHandlers>());

  const subscribe = useCallback<Subscribe>((handlers) => {
    listeners.current.add(handlers);
    return () => {
      listeners.current.delete(handlers);
    };
  }, []);

  // Read through the ref so adding a listener never reconnects the stream.
  const fanOut = useMemo<EventHandlers>(
    () => ({
      onTitle: (e) => {
        for (const l of listeners.current) l.onTitle?.(e);
      },
      onChannel: (e) => {
        for (const l of listeners.current) l.onChannel?.(e);
      },
      onSuggestion: (e) => {
        for (const l of listeners.current) l.onSuggestion?.(e);
      },
      onLlmPull: (e) => {
        for (const l of listeners.current) l.onLlmPull?.(e);
      },
      onJob: (e) => {
        for (const l of listeners.current) l.onJob?.(e);
      },
      // ⚠ THIS ONE WAS MISSING, and nothing could catch it: EventHandlers marks every
      // handler optional, so omitting a frame type type-checks perfectly. The core hook
      // subscribed to `fillerIngest`, this provider dropped it on the floor, and the ingest
      // panel — the frame's only consumer at the time — sat at "starting" forever because its
      // callback could never fire. A whole feature silently dead, with green everything.
      //
      // ⚠ That panel was retired in V38b (retired-ok), so this frame currently has NO renderer:
      // auto-fetch and queued downloads still emit progress and nothing shows it. Deliberately
      // kept rather than deleted — the fan-out is what a future progress surface subscribes to,
      // and the guard below is what stops it going missing again.
      //
      // The structural fix is the test below this file's own barrel: fanOutKeys() asserts
      // the provider fans out EVERY key the core EventHandlers declares, so a new frame
      // type cannot be added to core and forgotten here.
      onFillerIngest: (e) => {
        for (const l of listeners.current) l.onFillerIngest?.(e);
      },
      onFillerSplit: (e) => {
        for (const l of listeners.current) l.onFillerSplit?.(e);
      },
      onPlayout: (e) => {
        for (const l of listeners.current) l.onPlayout?.(e);
      },
      onActivity: (e) => {
        for (const l of listeners.current) l.onActivity?.(e);
      },
      onDatabase: (e) => {
        for (const l of listeners.current) l.onDatabase?.(e);
      },
    }),
    [],
  );
  useLoomarrEvents(fanOut);

  return <EventsContext.Provider value={subscribe}>{children}</EventsContext.Provider>;
};

// useLoomarrEventListener attaches handlers for the component's lifetime. Outside the
// provider it is a no-op rather than a throw: a component rendered in isolation (a
// story, a unit test) should still render, just without live frames.
const useLoomarrEventListener = (handlers: EventHandlers): void => {
  const subscribe = useContext(EventsContext);
  const ref = useRef(handlers);
  ref.current = handlers;

  useEffect(() => {
    if (!subscribe) return;
    return subscribe({
      onTitle: (e) => ref.current.onTitle?.(e),
      onChannel: (e) => ref.current.onChannel?.(e),
      onSuggestion: (e) => ref.current.onSuggestion?.(e),
      onLlmPull: (e) => ref.current.onLlmPull?.(e),
      onFillerIngest: (e) => ref.current.onFillerIngest?.(e),
      onFillerSplit: (e) => ref.current.onFillerSplit?.(e),
      onJob: (e) => ref.current.onJob?.(e),
      onActivity: (e) => ref.current.onActivity?.(e),
    });
  }, [subscribe]);
};

export { LoomarrEventsProvider, useLoomarrEventListener };
