import { createAuthenticatedFetch } from "@loomarr/core/pairing";
import {
  createRequestsController,
  createRequestsPort,
  type RequestsController,
  requestsNeedsYouCount,
} from "@loomarr/core/requests";
import { useEffect, useMemo, useSyncExternalStore } from "react";
import { AppState } from "react-native";

import type { PairedRequests, PairedRequestsOptions } from "./paired-requests.type";

/**
 * The phone's Requests on a paired server: one controller over the device's authenticated fetch, so
 * the tab's badge and the journey read the same snapshot. It loads on mount, not on first visit, so
 * an admin's badge is already counting when the app opens. The role comes from `/v1/auth/me` inside
 * the controller; a paired admin device is an admin (#1835), a member's never sees the admin lists.
 *
 * A request moves on the server, so the controller reads again when the device's stream announces
 * a change (#3352): a proposal waiting, approved or denied; a title or channel it brought settling;
 * and the progress of the person's own runs. Another member's run in progress is not this phone's
 * business until its proposal frame. It also reads again as the app returns to the foreground and
 * as the stream comes back, since the frames sent meanwhile are gone.
 */
const usePairedRequests = ({ credential, events, session }: PairedRequestsOptions): PairedRequests => {
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
  useEffect(() => {
    const reload = () => void controller.refresh();
    const ownRun = (jobId: string) =>
      controller.getSnapshot().entries.some(({ journey }) => journey.jobId === jobId);
    const unsubscribe = events.subscribe({
      handlers: {
        onChannel: reload,
        onProposal: reload,
        onSuggestion: ({ jobId }) => {
          if (ownRun(jobId)) reload();
        },
        onTitle: reload,
      },
      onResync: reload,
    });
    const foreground = AppState.addEventListener("change", (state) => {
      if (state === "active") reload();
    });
    return () => {
      unsubscribe();
      foreground.remove();
    };
  }, [controller, events]);
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  return { controller, needsYouCount: requestsNeedsYouCount(snapshot) };
};

export { usePairedRequests };
