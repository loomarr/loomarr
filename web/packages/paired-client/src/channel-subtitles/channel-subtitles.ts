import {
  type ChannelSubtitlesSource,
  nextSubtitle,
  SUBTITLES_OFF,
  type SubtitleChoice,
  subtitleChoices,
  useChannelSubtitles,
} from "@loomarr/core/channel-subtitles";
import type { PlayerSnapshot } from "@loomarr/player";
import { useCallback, useEffect, useRef, useState } from "react";

import type { LastChannelStorage } from "../last-channel";

// The viewer's subtitles on this device: a track key ("en", "en-sdh") or Off, default Off. It follows
// the viewer across channels and programmes, as the web's does per browser.
const SUBTITLES_KEY = "loomarr.subtitles";
const TRACK_KEY = /^[a-z0-9-]{1,40}$/;

interface PairedSubtitles {
  /** The lines on screen now, for PlayerChrome's `captions`. */
  captions: readonly string[];
  /** Choose a track key, or SUBTITLES_OFF; remembered on the device. */
  choose: (key: string) => void;
  /** Step to the next choice, round to Off (the web's C). */
  next: () => void;
  /** A track is chosen: the Subtitles key reads as on. */
  on: boolean;
  /** The Subtitles menu's rows. */
  options: readonly SubtitleChoice[];
}

/**
 * Subtitles on the phone and the TV: the remembered track, the menu's rows, and the lines along the
 * picture's own clock (the live clock's viewerTimeMs, so behind live reads what is on screen). Read
 * only while a channel plays. Storage is the app's SecureStore, as for the last channel.
 */
const usePairedSubtitles = (
  source: ChannelSubtitlesSource,
  storage: LastChannelStorage,
  snapshot: PlayerSnapshot,
): PairedSubtitles => {
  const [key, setKey] = useState(SUBTITLES_OFF);
  // A choice made before the stored one is read wins over it.
  const chosen = useRef(false);
  useEffect(() => {
    let live = true;
    storage.getItem(SUBTITLES_KEY).then(
      (stored) => {
        if (live && !chosen.current && stored && TRACK_KEY.test(stored)) setKey(stored);
      },
      // An unreadable store is the same as nothing remembered: Off.
      () => undefined,
    );
    return () => {
      live = false;
    };
  }, [storage]);
  const choose = useCallback(
    (next: string) => {
      chosen.current = true;
      setKey(next);
      void storage.setItem(SUBTITLES_KEY, next).catch(() => undefined);
    },
    [storage],
  );

  const playhead = snapshot.livePlayback?.viewerTimeMs ?? 0;
  const playing = snapshot.status === "playing" && snapshot.livePlayback !== undefined;
  const { lines, tracks } = useChannelSubtitles(source, snapshot.channel?.id, playhead, key, playing);
  const options = subtitleChoices(tracks ?? [], key);
  return {
    captions: lines,
    choose,
    next: () => choose(nextSubtitle(options)),
    on: key !== SUBTITLES_OFF,
    options,
  };
};

export type { PairedSubtitles };
export { usePairedSubtitles };
