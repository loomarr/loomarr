import { waitFor } from "storybook/test";

// Real offline playback is allowed to load, then frozen only for the workshop snapshot.
// Missing media must fail setup rather than bless an empty/error surface as a ready player.
const pauseStoryVideo = async (canvasElement: HTMLElement) => {
  const video = canvasElement.querySelector("video");
  if (!video) throw new Error("The exact clip preview must mount a video");
  await waitFor(() => {
    if (video.readyState < 2) throw new Error("Waiting for the offline clip fixture");
  });
  video.pause();
  video.currentTime = 0;
};

export { pauseStoryVideo };
