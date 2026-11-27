import { dispatchFrame } from "@loomarr/core/events";
import type { PairedEventSubscriber, PairedEventsHub } from "./paired-events.type";

/**
 * A device keeps one stream to its server, however many parts of the app follow it: each stream is
 * a long-lived request, and a phone's HTTP client allows only a few to one host at once. Each frame
 * goes to every listener, in the order they subscribed.
 */
const createPairedEvents = (): PairedEventsHub => {
  const subscribers = new Set<PairedEventSubscriber>();
  return {
    onFrame: (type, payload) => {
      for (const { handlers } of [...subscribers]) dispatchFrame(handlers, type, payload);
    },
    resync: () => {
      for (const { onResync } of [...subscribers]) onResync?.();
    },
    subscribe: (subscriber) => {
      subscribers.add(subscriber);
      return () => {
        subscribers.delete(subscriber);
      };
    },
  };
};

export { createPairedEvents };
