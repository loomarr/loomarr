import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { VideoPlayerChrome } from "@/components/loomarr/player/video-player";
import { ChromeBridge } from "./chrome-bridge";

const chrome = (muted: boolean, setMuted: (muted: boolean) => void): VideoPlayerChrome =>
  ({ muted, setMuted }) as unknown as VideoPlayerChrome;

describe("ChromeBridge", () => {
  it("hands the player's mute up as it changes, with the player's own setter, and draws nothing", () => {
    const setMuted = vi.fn();
    const onSound = vi.fn();
    const { container, rerender } = render(<ChromeBridge chrome={chrome(false, setMuted)} onSound={onSound} />);
    expect(container.childElementCount).toBe(0);
    expect(onSound).toHaveBeenLastCalledWith({ muted: false, setMuted });
    rerender(<ChromeBridge chrome={chrome(true, setMuted)} onSound={onSound} />);
    expect(onSound).toHaveBeenLastCalledWith({ muted: true, setMuted });
    onSound.mock.lastCall?.[0].setMuted(false);
    expect(setMuted).toHaveBeenCalledWith(false);
  });

  it("says nothing new while the sound has not changed", () => {
    const setMuted = vi.fn();
    const onSound = vi.fn();
    const { rerender } = render(<ChromeBridge chrome={chrome(false, setMuted)} onSound={onSound} />);
    rerender(<ChromeBridge chrome={chrome(false, setMuted)} onSound={onSound} />);
    expect(onSound).toHaveBeenCalledOnce();
  });
});
