import { useEffect } from "react";
import type { VideoPlayerChrome } from "@/components/loomarr/player/video-player";

// HideForIdent clears the controls as the station-ID card arrives, so the card stands alone as the
// mock draws it; moving the pointer brings the controls back in its place.
const HideForIdent = ({ hide, ident }: { hide: VideoPlayerChrome["hide"]; ident: boolean }) => {
  useEffect(() => {
    if (ident) hide();
  }, [hide, ident]);
  return null;
};

export { HideForIdent };
