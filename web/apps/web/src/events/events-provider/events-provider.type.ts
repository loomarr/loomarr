import type { FrameHandler } from "@loomarr/core/events";
import type { ReactNode } from "react";

// Subscribers get the same frames the app-wide stream already receives; unsubscribing
// is the returned cleanup.
type Subscribe = (onFrame: FrameHandler) => () => void;

interface LoomarrEventsProviderProps {
  children: ReactNode;
}

export type { LoomarrEventsProviderProps, Subscribe };
