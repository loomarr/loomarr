import { useCallback, useEffect, useState } from "react";

// How long a change of level or mute shows the volume level (§9.5).
const VOLUME_FLASH_MS = 1_500;

// The volume level (§9.5, desktop): opened by the Volume key in the menu's place, and shown for a
// moment when the level or mute changes.
const useVolumeLevel = () => {
  const [volumeOpen, setVolumeOpen] = useState(false);
  // A count of changes, not a flag: each change restarts the moment; zero is hidden.
  const [volumeFlash, setVolumeFlash] = useState(0);
  const flashVolume = useCallback(() => setVolumeFlash((n) => n + 1), []);
  useEffect(() => {
    if (volumeFlash === 0) return;
    const timer = window.setTimeout(() => setVolumeFlash(0), VOLUME_FLASH_MS);
    return () => window.clearTimeout(timer);
  }, [volumeFlash]);
  const closeVolume = () => {
    setVolumeOpen(false);
    setVolumeFlash(0);
  };
  return { closeVolume, flashVolume, setVolumeFlash, setVolumeOpen, volumeFlash, volumeOpen };
};

export { useVolumeLevel };
