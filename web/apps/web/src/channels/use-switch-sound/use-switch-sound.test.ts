import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { startChannelSwitchSound } from "../switch-sound";
import { useSwitchSound } from "./use-switch-sound";

vi.mock("../switch-sound", () => ({ startChannelSwitchSound: vi.fn() }));

type Props = Parameters<typeof useSwitchSound>[0];

describe("useSwitchSound", () => {
  const lock = vi.fn();
  let video: { muted: boolean };
  let props: Props;
  beforeEach(() => {
    vi.mocked(startChannelSwitchSound).mockReset().mockReturnValue({ lock });
    lock.mockReset();
    video = { muted: false };
    props = {
      channelId: "ch-1",
      heldFrame: true,
      tuning: false,
      video: { current: video as HTMLVideoElement },
    };
  });

  const render = () => renderHook((p: Props) => useSwitchSound(p), { initialProps: props });

  it("stays quiet on the first tune, and on a change before any picture has played", () => {
    const hook = renderHook((p: Props) => useSwitchSound(p), {
      initialProps: { ...props, heldFrame: false },
    });
    hook.rerender({ ...props, heldFrame: false, channelId: "ch-2", tuning: true });
    expect(startChannelSwitchSound).not.toHaveBeenCalled();
  });

  it("plays on a channel change, muting the programme until the new channel locks", () => {
    const hook = render();
    hook.rerender({ ...props, channelId: "ch-2", tuning: true });
    expect(startChannelSwitchSound).toHaveBeenCalledWith(video, { enabled: true });
    expect(video.muted).toBe(true);

    hook.rerender({ ...props, channelId: "ch-2", tuning: false });
    expect(lock).toHaveBeenCalledTimes(1);
    expect(video.muted).toBe(false);
  });

  it("leaves a programme the viewer muted muted", () => {
    video.muted = true;
    const hook = render();
    hook.rerender({ ...props, channelId: "ch-2", tuning: true });
    hook.rerender({ ...props, channelId: "ch-2", tuning: false });
    expect(video.muted).toBe(true);
  });

  it("cuts the running static when the viewer surfs on, and when the player goes", () => {
    const hook = render();
    hook.rerender({ ...props, channelId: "ch-2", tuning: true });
    hook.rerender({ ...props, channelId: "ch-3", tuning: true });
    expect(startChannelSwitchSound).toHaveBeenCalledTimes(2);
    expect(lock).toHaveBeenCalledTimes(1);
    hook.unmount();
    expect(lock).toHaveBeenCalledTimes(2);
  });

  it("does not mute the programme when the gate declines the sound", () => {
    vi.mocked(startChannelSwitchSound).mockReturnValue(undefined);
    const hook = render();
    hook.rerender({ ...props, channelId: "ch-2", tuning: true });
    expect(video.muted).toBe(false);
  });
});
