import type { EventHandlers, FrameHandler } from "@loomarr/core/events";

/** One listener on the paired server's event stream. */
interface PairedEventSubscriber {
  /** The frames it follows. */
  handlers: EventHandlers;
  /** Runs as the stream comes back after a drop: the frames sent while it was down are gone. */
  onResync?: () => void;
}

/** The paired server's one event stream, shared by everything on the device that follows it. */
interface PairedEvents {
  /** Adds a listener; the returned function removes it. */
  subscribe: (subscriber: PairedEventSubscriber) => () => void;
}

/** The stream's side of the fan-out: each frame and each reconnect, handed to every listener. */
interface PairedEventsHub extends PairedEvents {
  onFrame: FrameHandler;
  resync: () => void;
}

export type { PairedEventSubscriber, PairedEvents, PairedEventsHub };
