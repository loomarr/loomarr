import { waitFor } from "storybook/test";

// Real offline playback is allowed to load, then frozen only for the workshop snapshot.
// Missing media must fail setup rather than bless an empty/error surface as a ready player.
const pauseStoryVideo = async (canvasElement: HTMLElement) => {
  const video = canvasElement.querySelector("video");
  if (!video) throw new Error("The exact clip preview must mount a video");
  await waitFor(() => {
    if (video.readyState < 2) throw new Error("Waiting for the offline clip fixture");
    // The fixture's header has no duration: it reads Infinity until the browser has parsed to the
    // end, and the readout shows "/ 0:00" until then (#2006).
    if (!Number.isFinite(video.duration)) throw new Error("Waiting for the clip's length");
  });
  video.pause();
  // A seek is asynchronous: the clock shows wherever playback had reached (0:01 on a slow
  // runner) until `seeked` fires, so a shot taken before it is not frame-stable (#1956).
  const seeked = new Promise((resolve) => video.addEventListener("seeked", resolve, { once: true }));
  video.currentTime = 0;
  await seeked;
};

export { pauseStoryVideo };
