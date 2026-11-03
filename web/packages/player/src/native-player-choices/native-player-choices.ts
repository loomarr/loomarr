import { type AudioChoiceOption, audioChoice } from "@loomarr/core/audio-choice";
import { useEffect } from "react";

import { cueLinesAt, nextSubtitles, subtitleLabel } from "../live-subtitles";
import { useNativeVideoHeight } from "../native/native";
import type { PairedClient } from "../paired-client";
import { usePairedSubtitles } from "../paired-subtitles";
import { rungLabel } from "../playback-info";
import {
  type PlayerPreferenceStorage,
  useAudioPreference,
  useSubtitlesPreference,
} from "../player-preferences";

// What a native player offers the viewer and draws over the picture, shared by the phone and the TV:
// Settings' quality (Auto, the platform's pick, naming the rung on air), the programme's audio
// (#2088 decision 6) and subtitles (decision 5) at the playhead with the viewer's remembered choice
// of each, the chosen track's lines for the caption (the player takes no sidecar text track), and the
// transport's Subtitles step. Plain data: each app draws it at its own density.

interface NativePlayerChoice {
  label: string;
  selected: boolean;
  value: string;
}

interface NativePlayerChoices {
  /** The channel's default and the programme's alternate; a choice only with `onSelect`. */
  audio: { onSelect?: (value: string) => void; options: readonly AudioChoiceOption[] };
  /** The chosen track's lines at the playhead. */
  captionLines: readonly string[];
  /** Steps Off → each track → Off; omitted while the programme carries none. */
  nextSubtitles?: () => void;
  /** Auto alone, read-only: the platform player picks the rung itself (#2127). */
  quality: { options: readonly NativePlayerChoice[] };
  /** Off and each track; omitted while the programme carries none. */
  subtitles?: { onSelect: (value: string) => void; options: readonly NativePlayerChoice[] };
  subtitlesOn: boolean;
}

interface NativePlayerChoicesInput {
  client: Pick<PairedClient, "subtitlesSource" | "tracks" | "transport">;
  /** The tuned channel, while it plays: subtitles load for it alone. */
  playingChannelId: string | undefined;
  /** Where the viewer's choices are remembered; keep it stable across renders. */
  storage: PlayerPreferenceStorage;
  viewerTimeMs: number;
}

const useNativePlayerChoices = ({
  client,
  playingChannelId,
  storage,
  viewerTimeMs,
}: NativePlayerChoicesInput): NativePlayerChoices => {
  const { subtitlesSource, tracks, transport } = client;
  const [audioPreference, setAudioPreference] = useAudioPreference(storage);
  const audio = audioChoice(tracks, audioPreference);
  useEffect(() => transport.setAlternateAudio(audio.alternate), [audio.alternate, transport]);
  const videoHeight = useNativeVideoHeight(transport);
  const [subtitlesChoice, setSubtitles] = useSubtitlesPreference(storage);
  const subtitles = usePairedSubtitles(subtitlesSource, playingChannelId, viewerTimeMs, subtitlesChoice);
  const carried = subtitles.tracks.length > 0;
  return {
    audio:
      audio.options.length > 1
        ? { onSelect: setAudioPreference, options: audio.options }
        : { options: audio.options },
    captionLines: cueLinesAt(subtitles.cues, viewerTimeMs),
    nextSubtitles: carried ? () => setSubtitles(nextSubtitles(subtitles.tracks, subtitlesChoice)) : undefined,
    quality: {
      options: [
        { label: videoHeight ? `Auto · ${rungLabel(videoHeight)}` : "Auto", selected: true, value: "auto" },
      ],
    },
    subtitles: carried
      ? {
          onSelect: (value) =>
            setSubtitles(value === "off" ? "off" : (subtitles.tracks[Number(value)] ?? "off")),
          options: [
            { label: "Off", selected: subtitles.active < 0, value: "off" },
            ...subtitles.tracks.map((track, i) => ({
              label: subtitleLabel(track),
              selected: i === subtitles.active,
              value: String(i),
            })),
          ],
        }
      : undefined,
    subtitlesOn: subtitles.active >= 0,
  };
};

export type { NativePlayerChoice, NativePlayerChoices, NativePlayerChoicesInput };
export { useNativePlayerChoices };
