import type { AudioChoice, ChannelAudioSource } from "@loomarr/core/channel-audio";
import { useEffect, useState } from "react";

/**
 * The Audio menu's rows on the phone and the TV: read each time the menu opens, for the channel on
 * air, so a programme that rolled over since the last look is what it lists. A read that fails or is
 * overtaken (the menu closed, the channel changed) leaves no rows rather than another channel's.
 */
const useChannelAudio = (
  source: ChannelAudioSource,
  channelId: string | undefined,
  open: boolean,
): readonly AudioChoice[] | undefined => {
  const [loaded, setLoaded] = useState<{ channelId: string; choices: AudioChoice[] }>();
  useEffect(() => {
    if (!open || !channelId) return;
    const pending = new AbortController();
    source.load(channelId, pending.signal).then(
      (choices) => {
        if (!pending.signal.aborted) setLoaded({ channelId, choices });
      },
      () => {
        if (!pending.signal.aborted) setLoaded(undefined);
      },
    );
    return () => pending.abort();
  }, [channelId, open, source]);
  return open && loaded && loaded.channelId === channelId ? loaded.choices : undefined;
};

export { useChannelAudio };
