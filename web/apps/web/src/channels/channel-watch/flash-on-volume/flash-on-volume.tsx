import { useEffect, useRef } from "react";

// FlashOnVolume shows the volume level for a moment when the level or mute changes after mount.
// The session's first play mutes itself (quiet): the unmute pill says so, and the level stays away.
const FlashOnVolume = ({
  muted,
  onChange,
  quiet,
  volume,
}: {
  muted: boolean;
  onChange: () => void;
  quiet: boolean;
  volume: number;
}) => {
  const seen = useRef({ muted, volume });
  useEffect(() => {
    if (seen.current.muted === muted && seen.current.volume === volume) return;
    seen.current = { muted, volume };
    if (!quiet) onChange();
  }, [muted, volume, onChange, quiet]);
  return null;
};

export { FlashOnVolume };
