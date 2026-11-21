import { type FrameHandler, openFrameStream } from "@loomarr/core/events";
import type { PairingCredential, PairingSession } from "@loomarr/core/pairing";
import { createNativeEventStreamFactory } from "@loomarr/player/native";
import { useEffect } from "react";
import { AppState } from "react-native";

interface UseEventStreamOptions {
  credential: Pick<PairingCredential, "serverUrl" | "token">;
  /** Every frame the server sends. Keep it stable: a new function reopens the stream. */
  onFrame: FrameHandler;
  /** Runs each time the stream comes back while the app stays in the foreground. */
  onReconnect: () => void;
  session: PairingSession;
}

/**
 * The server's live event stream, open only while the app is in the foreground. Frames sent while
 * it was down are gone, so `onReconnect` runs each time it comes back. Its first open after launch
 * or a return to the foreground calls nothing: whoever follows the stream reads again on those
 * itself.
 */
const useEventStream = ({ credential, onFrame, onReconnect, session }: UseEventStreamOptions) => {
  useEffect(() => {
    const createStream = createNativeEventStreamFactory({
      headers: { Authorization: `Bearer ${credential.token}` },
      onUnauthorized: () => session.revoked(),
    });
    let closeStream: (() => void) | undefined;
    const openStream = () => {
      if (closeStream) return;
      let opens = 0;
      closeStream = openFrameStream(
        onFrame,
        new URL("/v1/events", credential.serverUrl).toString(),
        createStream,
        () => {
          if (opens++ > 0) onReconnect();
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
  }, [credential.serverUrl, credential.token, onFrame, onReconnect, session]);
};

export type { UseEventStreamOptions };
export { useEventStream };
