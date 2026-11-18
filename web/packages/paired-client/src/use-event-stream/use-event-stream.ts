import { type EventHandlers, openEventStream } from "@loomarr/core/events";
import type { PairingCredential, PairingSession } from "@loomarr/core/pairing";
import { createNativeEventStreamFactory } from "@loomarr/player/native";
import { useEffect } from "react";
import { AppState } from "react-native";

interface UseEventStreamOptions {
  credential: Pick<PairingCredential, "serverUrl" | "token">;
  /** The frames to follow. Keep it stable: a new object reopens the stream. */
  handlers: EventHandlers;
  /** Called each time the stream comes back, for the frames sent while it was down. */
  onReload: () => void;
  session: PairingSession;
}

/**
 * The server's live event stream, open only while the app is in the foreground. Frames sent while
 * it was down are gone, so each time it comes back `onReload` runs; its first open needs none, as
 * the loads run as it opens, on launch and on every return to the foreground.
 */
const useEventStream = ({ credential, handlers, onReload, session }: UseEventStreamOptions) => {
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
        handlers,
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
  }, [credential.serverUrl, credential.token, handlers, onReload, session]);
};

export type { UseEventStreamOptions };
export { useEventStream };
