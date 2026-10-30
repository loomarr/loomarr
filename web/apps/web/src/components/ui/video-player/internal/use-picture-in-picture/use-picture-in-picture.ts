import { type RefObject, useCallback, useEffect, useState } from "react";

// usePictureInPicture owns the player's picture-in-picture (#2088 decision 4): whether the browser
// offers it, whether this <video> is in it now, and a toggle. Like useFullscreen it tracks the
// browser's OWN events, so closing the floating window flips the state too. A browser without the
// API (Firefox) reports unsupported, and the chrome draws no button and answers no key.
const usePictureInPicture = (videoRef: RefObject<HTMLVideoElement | null>) => {
  const supported = typeof document !== "undefined" && document.pictureInPictureEnabled === true;
  const [active, setActive] = useState(false);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !supported) return;
    const enter = () => setActive(true);
    const leave = () => setActive(false);
    video.addEventListener("enterpictureinpicture", enter);
    video.addEventListener("leavepictureinpicture", leave);
    return () => {
      video.removeEventListener("enterpictureinpicture", enter);
      video.removeEventListener("leavepictureinpicture", leave);
    };
  }, [videoRef, supported]);

  // Best-effort: the request needs a user gesture (a click or key) and a video with a frame.
  const togglePictureInPicture = useCallback(() => {
    if (document.pictureInPictureElement) {
      void document.exitPictureInPicture().catch(() => {});
    } else {
      void videoRef.current?.requestPictureInPicture().catch(() => {});
    }
  }, [videoRef]);

  return { pictureInPicture: active, pictureInPictureSupported: supported, togglePictureInPicture };
};

export { usePictureInPicture };
