import { useEffect, useMemo } from "react";

import type { PlayerController } from "../player-controller";

type ShellPauseController = Pick<PlayerController, "getSnapshot" | "pause" | "play">;

interface ShellPause {
  /** The viewer returned to the picture: resumes only what the shell paused, if it is still paused. */
  enter: () => void;
  /** The viewer chose something else to watch, so the old pause is no longer the shell's to undo. */
  forget: () => void;
  /** The picture is off screen: pauses it, remembering the channel if it was running. */
  leave: () => void;
}

/**
 * Pauses a stream the viewer can no longer see and resumes it on return, but only the stream the
 * shell itself paused: a viewer who tuned another channel meanwhile, or a controller that was
 * rebuilt, is left alone so the old transport never blips back to life.
 */
const createShellPause = (controller: ShellPauseController): ShellPause => {
  let pausedChannelId: string | undefined;
  return {
    enter: () => {
      const intent = pausedChannelId;
      pausedChannelId = undefined;
      if (!intent) return;
      const { channel, status } = controller.getSnapshot();
      if (status === "paused" && channel?.id === intent) void controller.play();
    },
    forget: () => {
      pausedChannelId = undefined;
    },
    leave: () => {
      const { channel, status } = controller.getSnapshot();
      // Already paused (by the shell, earlier) keeps the intent; only a running stream sets it.
      if (channel && (status === "playing" || status === "tuning")) pausedChannelId = channel.id;
      controller.pause();
    },
  };
};

/** Pauses the controller while `watching` is false and resumes it on return; `forget` drops the resume. */
const useShellPause = (controller: ShellPauseController, watching: boolean): (() => void) => {
  const shellPause = useMemo(() => createShellPause(controller), [controller]);
  useEffect(() => {
    if (watching) shellPause.enter();
    else shellPause.leave();
  }, [shellPause, watching]);
  return shellPause.forget;
};

export type { ShellPause, ShellPauseController };
export { createShellPause, useShellPause };
