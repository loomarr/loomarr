/**
 * Watches one element for the first decoded frame of a tune and calls `onFirstFrame` when it paints.
 *
 * Where the browser reports decoded frames (`requestVideoFrameCallback`) the watch is armed once the
 * element has data, so a callback requested before the source attached cannot fire for the outgoing
 * channel. Elsewhere the first `playing` stands in for the frame.
 */
const createFirstFrameWatch = (el: HTMLVideoElement, onFirstFrame: () => void) => {
  let frameCallback: number | undefined;
  let firstFrameWatchArmed = false;
  const requestFrame = (
    el as Partial<Pick<HTMLVideoElement, "requestVideoFrameCallback">>
  ).requestVideoFrameCallback?.bind(el);
  const arm = () => {
    if (firstFrameWatchArmed) return;
    firstFrameWatchArmed = true;
    if (requestFrame) frameCallback = requestFrame(() => onFirstFrame());
    else el.addEventListener("playing", onFirstFrame, { once: true });
  };
  let joinOnLoadedMetadata: (() => void) | undefined;
  let joinOnLoadedData: (() => void) | undefined;
  let joinOnCanPlay: (() => void) | undefined;
  const onLoadedMetadata = () => {
    const join = joinOnLoadedMetadata;
    joinOnLoadedMetadata = undefined;
    join?.();
  };
  const onLoadedData = () => {
    arm();
    const join = joinOnLoadedData;
    joinOnLoadedData = undefined;
    join?.();
  };
  const onCanPlay = () => {
    const join = joinOnCanPlay;
    joinOnCanPlay = undefined;
    join?.();
  };

  return {
    arm,
    /** Runs `join` once on the element's first loaded metadata, loaded data and canplay each. */
    joinWhenLoaded: (join: () => void) => {
      joinOnLoadedMetadata = join;
      joinOnLoadedData = join;
      joinOnCanPlay = join;
    },
    /** Arms the watch on loaded data when frames are reported, and at once when they are not. */
    listenForLoadedData: () => {
      if (requestFrame) el.addEventListener("loadeddata", onLoadedData, { once: true });
      else arm();
    },
    /** As `listenForLoadedData`, and also runs the `joinWhenLoaded` join on metadata and canplay. */
    listenForLoad: () => {
      if (requestFrame) {
        el.addEventListener("loadedmetadata", onLoadedMetadata, { once: true });
        el.addEventListener("loadeddata", onLoadedData, { once: true });
        el.addEventListener("canplay", onCanPlay, { once: true });
      } else arm();
    },
    stop: () => {
      joinOnLoadedMetadata = undefined;
      joinOnLoadedData = undefined;
      joinOnCanPlay = undefined;
      if (frameCallback !== undefined) el.cancelVideoFrameCallback?.(frameCallback);
      el.removeEventListener("loadedmetadata", onLoadedMetadata);
      el.removeEventListener("loadeddata", onLoadedData);
      el.removeEventListener("canplay", onCanPlay);
      el.removeEventListener("playing", onFirstFrame);
    },
  };
};

export { createFirstFrameWatch };
