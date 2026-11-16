import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { VideoPlayerChrome } from "@/components/loomarr/player/video-player";
import { FirstPlayMuted } from "./first-play-muted";

const chromeOf = (over: Partial<VideoPlayerChrome> = {}) =>
  ({ muted: false, setMuted: vi.fn(), ...over }) as unknown as VideoPlayerChrome;

beforeEach(() => sessionStorage.clear());
afterEach(() => sessionStorage.clear());

describe("FirstPlayMuted", () => {
  it("mutes the session's first play once and says it started", () => {
    const chrome = chromeOf();
    const onStart = vi.fn();
    const { rerender } = render(<FirstPlayMuted chrome={chrome} onStart={onStart} onUnmuted={vi.fn()} />);

    expect(chrome.setMuted).toHaveBeenCalledWith(true);
    expect(onStart).toHaveBeenCalledTimes(1);

    rerender(<FirstPlayMuted chrome={chrome} onStart={onStart} onUnmuted={vi.fn()} />);
    expect(chrome.setMuted).toHaveBeenCalledTimes(1);
  });

  it("leaves a later mount in the same session alone", () => {
    sessionStorage.setItem("loomarr.player.first-play", "1");
    const chrome = chromeOf();
    const onStart = vi.fn();

    render(<FirstPlayMuted chrome={chrome} onStart={onStart} onUnmuted={vi.fn()} />);

    expect(chrome.setMuted).not.toHaveBeenCalled();
    expect(onStart).not.toHaveBeenCalled();
  });

  it("says the sound came on once it was heard muted and is not any more", () => {
    const onUnmuted = vi.fn();
    const { rerender } = render(
      <FirstPlayMuted chrome={chromeOf({ muted: true })} onStart={vi.fn()} onUnmuted={onUnmuted} />,
    );
    expect(onUnmuted).not.toHaveBeenCalled();

    rerender(<FirstPlayMuted chrome={chromeOf({ muted: false })} onStart={vi.fn()} onUnmuted={onUnmuted} />);

    expect(onUnmuted).toHaveBeenCalledTimes(1);
  });
});
