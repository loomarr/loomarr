import { openEventStream } from "@loomarr/core/events";
import type { PairingCredential, PairingSession } from "@loomarr/core/pairing";
import { createNativeEventStreamFactory } from "@loomarr/player/native";
import { useEffect } from "react";
import { AppState } from "react-native";

interface UseEventStreamOptions {
  credential: Pick<PairingCredential, "serverUrl" | "token">;
  /** Called for a channel change the server announces, and each time the stream comes back. */
  onReload: () => void;
  session: PairingSession;
}

/**
 * The server's live event stream, open only while the app is in the foreground. Frames sent while
 * it was down are gone, so each time it comes back `onReload` runs; its first open needs none, as
 * the loads run as it opens, on launch and on every return to the foreground.
 */
const useEventStream = ({ credential, onReload, session }: UseEventStreamOptions) => {
  useEffect(() => {
    const createStream = createNativeEventStreamFactory({
      headers: { Authorization: `Bearer ${credential.token}` },
      onUnauthorized: () => session.revoked(),
    });
    let closeStream: (() => void) | undefined;
    const openStream = () => {
      if (closeStream) return;
      let opens = 0;
      closeStream = openEventStream(
        { onChannel: onReload },
        new URL("/v1/events", credential.serverUrl).toString(),
        createStream,
        () => {
          if (opens++ > 0) onReload();
        },
      );
    };
    const closeActiveStream = () => {
      closeStream?.();
      closeStream = undefined;
    };
    if (AppState.currentState === "active") openStream();
    const subscription = AppState.addEventListener("change", (state) => {
      if (state === "active") openStream();
      else closeActiveStream();
    });
    return () => {
      subscription.remove();
      closeActiveStream();
    };
  }, [credential.serverUrl, credential.token, onReload, session]);
};

export type { UseEventStreamOptions };
export { useEventStream };
