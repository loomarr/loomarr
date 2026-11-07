import { dispatchFrame, type EventHandlers, type FrameHandler, useLoomarrEvents } from "@loomarr/core/events";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef } from "react";
import type { LoomarrEventsProviderProps, Subscribe } from "./events-provider.type";

// ONE SSE stream, many listeners. `useLoomarrEvents` opens an EventSource, and the app
// layout already mounts it for cache invalidation — so a screen that also wants the
// frame *payload* (the Suggest workspace needs the searching/reasoning/scoring phases,
// which exist only on the wire) must not call the hook again and open a second stream.
// This provider owns the single subscription and fans every frame out to whoever asked.
//
// No frame type is named here. Core's dispatcher hands over (type, payload) for every type the
// spec declares and `dispatchFrame` picks the listener's handler by that type, so a new frame
// reaches listeners without touching this file.
const EventsContext = createContext<Subscribe | undefined>(undefined);

const LoomarrEventsProvider = ({ children }: LoomarrEventsProviderProps) => {
  const listeners = useRef(new Set<FrameHandler>());

  const subscribe = useCallback<Subscribe>((onFrame) => {
    listeners.current.add(onFrame);
    return () => {
      listeners.current.delete(onFrame);
    };
  }, []);

  // Read through the ref so adding a listener never reconnects the stream.
  const fanOut = useMemo<FrameHandler>(
    () => (type, payload) => {
      for (const listener of listeners.current) listener(type, payload);
    },
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
    return subscribe((type, payload) => dispatchFrame(ref.current, type, payload));
  }, [subscribe]);
};

export { LoomarrEventsProvider, useLoomarrEventListener };
