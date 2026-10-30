import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { usePictureInPicture } from "./use-picture-in-picture";

// jsdom implements no Picture-in-Picture API, so the document flags and the element method are
// stubbed. What is OURS to test: support is read from the browser, the toggle requests vs exits by
// the browser's current state, and the video's own enter/leave events drive the state (so closing
// the floating window flips it back).
describe("usePictureInPicture", () => {
  const setDocument = (enabled: boolean, element: Element | null) => {
    Object.defineProperty(document, "pictureInPictureEnabled", { value: enabled, configurable: true });
    Object.defineProperty(document, "pictureInPictureElement", { value: element, configurable: true });
  };
  afterEach(() => {
    vi.restoreAllMocks();
    setDocument(false, null);
  });

  const videoRef = () => {
    const video = document.createElement("video");
    video.requestPictureInPicture = vi.fn().mockResolvedValue({});
    return { current: video };
  };

  it("reports unsupported where the browser has no picture-in-picture", () => {
    setDocument(false, null);
    const { result } = renderHook(() => usePictureInPicture(videoRef()));
    expect(result.current.pictureInPictureSupported).toBe(false);
  });

  it("requests picture-in-picture on the video when none is open, and exits when one is", () => {
    setDocument(true, null);
    const ref = videoRef();
    const { result } = renderHook(() => usePictureInPicture(ref));
    expect(result.current.pictureInPictureSupported).toBe(true);

    act(() => result.current.togglePictureInPicture());
    expect(ref.current.requestPictureInPicture).toHaveBeenCalledOnce();

    setDocument(true, ref.current);
    const exit = vi.fn().mockResolvedValue(undefined);
    document.exitPictureInPicture = exit;
    act(() => result.current.togglePictureInPicture());
    expect(exit).toHaveBeenCalledOnce();
    expect(ref.current.requestPictureInPicture).toHaveBeenCalledOnce();
  });

  it("tracks the video's enter and leave events (closing the floating window flips it back)", () => {
    setDocument(true, null);
    const ref = videoRef();
    const { result } = renderHook(() => usePictureInPicture(ref));
    expect(result.current.pictureInPicture).toBe(false);

    act(() => {
      ref.current.dispatchEvent(new Event("enterpictureinpicture"));
    });
    expect(result.current.pictureInPicture).toBe(true);
    act(() => {
      ref.current.dispatchEvent(new Event("leavepictureinpicture"));
    });
    expect(result.current.pictureInPicture).toBe(false);
  });
});
