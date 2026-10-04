import { createAuthenticatedFetch } from "@loomarr/core/pairing";
import {
  createRequestsController,
  createRequestsPort,
  type RequestsController,
  requestsNeedsYouCount,
} from "@loomarr/core/requests";
import { useEffect, useMemo, useSyncExternalStore } from "react";

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
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  return { controller, needsYouCount: requestsNeedsYouCount(snapshot) };
};

export { usePairedRequests };
