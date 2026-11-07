import { type RefObject, useCallback, useEffect, useRef } from "react";
import { type SwitchSound, startChannelSwitchSound } from "../switch-sound";
import { useSwitchSoundPreference } from "../use-switch-sound-preference";

type SwitchSoundOptions = {
  // The tuned channel: a change is what starts the sound.
  channelId: string;
  // A picture has played here, so a change is a channel change (not a cold start).
  heldFrame: boolean;
  // The new channel is still tuning; its first frame (or a failed tune) is the lock.
  tuning: boolean;
  video: RefObject<HTMLVideoElement | null>;
};

// The channel-change sound (#1620), a dial set: a clunk, then static until the new channel's first
// frame, cut at lock. It starts as the tuned channel changes — the moment the viewer asks, not when
// transport catches up — and only once a picture has played here: a cold start is not a channel
// change, and an autoplayed first tune has had no gesture. The switch-sound module owns the rest of
// the gate (the viewer's preference, the player's mute and volume, the page's user activation).
//
// While the static plays, the programme's own audio is muted, as a set's is; it comes back at lock.
// That is a mute of the element alone: the player's own mute state is untouched, so its button
// still shows the viewer's choice, and the element is only unmuted again if the static muted it.
//
// Returns the viewer's preference and its setter, for Settings.
const useSwitchSound = ({ channelId, heldFrame, tuning, video }: SwitchSoundOptions) => {
  const [on, setOn] = useSwitchSoundPreference();
  const soundedChannelRef = useRef(channelId);
  const runningRef = useRef<{ sound: SwitchSound; programmeMuted: boolean } | undefined>(undefined);

  const end = useCallback(() => {
    const running = runningRef.current;
    if (!running) return;
    runningRef.current = undefined;
    running.sound.lock();
    if (video.current?.muted) video.current.muted = running.programmeMuted;
  }, [video]);

  useEffect(() => {
    if (soundedChannelRef.current === channelId) return;
    soundedChannelRef.current = channelId;
    const element = video.current;
    if (!heldFrame || !element) return;
    // Surfing on while the static still runs: that one ends, and the new clunk takes over.
    end();
    const sound = startChannelSwitchSound(element, { enabled: on });
    if (!sound) return;
    runningRef.current = { sound, programmeMuted: element.muted };
    element.muted = true;
  }, [channelId, heldFrame, on, end, video]);

  // Lock: the first frame has decoded (or the tune failed), so the static cuts off.
  useEffect(() => {
    if (!tuning) end();
  }, [tuning, end]);
  useEffect(() => end, [end]);

  return [on, setOn] as const;
};

export { useSwitchSound };
