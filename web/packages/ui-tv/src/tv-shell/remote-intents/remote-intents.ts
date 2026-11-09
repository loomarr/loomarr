import type { ClientDestination } from "@loomarr/ui";

import type { TvRemote } from "../../tv-remote";
import type { TvWatchingRemoteIntent } from "../../watching-navigation";

/** The part of the player controller the remote drives. */
interface TvRemoteController {
  getSnapshot: () => { channel?: unknown; status: string };
  pause: () => void;
  play: () => Promise<unknown>;
  step: (direction: -1 | 1) => Promise<unknown>;
  tuneChannel: (channelId: string) => Promise<unknown>;
  tuneNumber: (digits: string) => Promise<unknown>;
}

/** Does what a remote press on Watching asked for. */
const runTvRemoteIntent = (
  intent: TvWatchingRemoteIntent | undefined,
  controller: TvRemoteController,
  open: (destination: ClientDestination) => void,
) => {
  switch (intent?.kind) {
    case "step":
      void controller.step(intent.direction);
      break;
    case "tune-number":
      void controller.tuneNumber(intent.digits);
      break;
    case "open-guide":
      open("guide");
      break;
    case "open-surf":
      open("surf");
      break;
    case "toggle-pause": {
      // The Watching controls' own transport: Play when paused, Pause otherwise.
      const { channel, status } = controller.getSnapshot();
      if (!channel) break;
      if (status === "paused") void controller.play();
      else controller.pause();
      break;
    }
  }
};

/**
 * What picking a channel in the Guide or Surf does: tune it, restart the Watching controls' timer and
 * show Watching. tvOS also delivers the held row's own press when the finger lifts after a long
 * press; that is not a tune request.
 */
const createTvListTune =
  (
    remote: Pick<TvRemote, "consumeLongSelectRelease">,
    controller: Pick<TvRemoteController, "tuneChannel">,
    showControlsForActivity: () => void,
    open: (destination: ClientDestination) => void,
  ) =>
  (channelId: string) => {
    if (remote.consumeLongSelectRelease()) return;
    void controller.tuneChannel(channelId);
    showControlsForActivity();
    open("watching");
  };

export type { TvRemoteController };
export { createTvListTune, runTvRemoteIntent };
