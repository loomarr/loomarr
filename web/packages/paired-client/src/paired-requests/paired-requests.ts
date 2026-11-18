import { createAuthenticatedFetch } from "@loomarr/core/pairing";
import {
  createRequestsController,
  createRequestsPort,
  type RequestsController,
  requestsNeedsYouCount,
} from "@loomarr/core/requests";
import { useCallback, useEffect, useMemo, useSyncExternalStore } from "react";
import { useEventStream } from "../use-event-stream";

import type { PairedRequests, PairedRequestsOptions } from "./paired-requests.type";

/**
 * The phone's Requests on a paired server: one controller over the device's authenticated fetch, so
 * the tab's badge and the journey read the same snapshot. It loads on mount, not on first visit, so
 * an admin's badge is already counting when the app opens. The role comes from `/v1/auth/me` inside
 * the controller; a paired admin device is an admin (#1835), a member's never sees the admin lists.
 */
const usePairedRequests = ({ credential, session }: PairedRequestsOptions): PairedRequests => {
  const controller: RequestsController = useMemo(
    () =>
      createRequestsController({
        port: createRequestsPort(createAuthenticatedFetch(credential, () => session.revoked())),
      }),
    [credential, session],
  );
  useEffect(() => {
    void controller.refresh();
    return () => controller.dispose();
  }, [controller]);
  // A request moves on the server, so it is read again on the frames that announce it (#3352): a
  // run's progress and outcome, a title or approval changing, and an approved request's channel.
  const reload = useCallback(() => void controller.refresh(), [controller]);
  const requestFrames = useMemo(
    () => ({ onChannel: reload, onSuggestion: reload, onTitle: reload }),
    [reload],
  );
  useEventStream({ credential, handlers: requestFrames, onReload: reload, session });
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  return { controller, needsYouCount: requestsNeedsYouCount(snapshot) };
};

export { usePairedRequests };
