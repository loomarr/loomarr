import { handleTvWatchingRemoteEvent, type TvWatchingRemotePort } from "@loomarr/ui-tv";
import { beforeEach, describe, expect, it, vi } from "vitest";

const remotePort = (): TvWatchingRemotePort => ({
  commitNumber: vi.fn(),
  enterNumber: vi.fn(() => false),
  openGuide: vi.fn(),
  openSurf: vi.fn(),
  pause: vi.fn(),
  play: vi.fn(),
  revealOverlay: vi.fn(),
  step: vi.fn(),
  togglePlayback: vi.fn(),
});

describe("TV Watching remote", () => {
  let port: TvWatchingRemotePort;

  beforeEach(() => {
    port = remotePort();
  });

  it("opens Guide when OK is pressed without a pending channel number", () => {
    handleTvWatchingRemoteEvent("select", false, port);

    expect(port.openGuide).toHaveBeenCalledOnce();
    expect(port.commitNumber).not.toHaveBeenCalled();
  });

  it("commits pending number entry instead of opening Guide", () => {
    handleTvWatchingRemoteEvent("select", true, port);

    expect(port.commitNumber).toHaveBeenCalledOnce();
    expect(port.openGuide).not.toHaveBeenCalled();
  });

  it.each([
    ["up", 1],
    ["channelUp", 1],
    ["down", -1],
    ["channelDown", -1],
  ] as const)("maps %s to an adjacent-channel step", (eventType, direction) => {
    handleTvWatchingRemoteEvent(eventType, false, port);

    expect(port.step).toHaveBeenCalledWith(direction);
  });

  it.each(["left", "menu"])("maps %s to Surf", (eventType) => {
    handleTvWatchingRemoteEvent(eventType, false, port);

    expect(port.openSurf).toHaveBeenCalledOnce();
  });

  it("lets number entry consume digits and reveals the channel buffer", () => {
    vi.mocked(port.enterNumber).mockReturnValue(true);

    handleTvWatchingRemoteEvent("4", false, port);

    expect(port.revealOverlay).toHaveBeenCalledOnce();
    expect(port.openGuide).not.toHaveBeenCalled();
  });

  it.each([
    ["play", "play"],
    ["pause", "pause"],
    ["playPause", "togglePlayback"],
  ] as const)("maps %s to the native playback controller", (eventType, action) => {
    handleTvWatchingRemoteEvent(eventType, false, port);

    expect(port[action]).toHaveBeenCalledOnce();
    expect(port.revealOverlay).toHaveBeenCalledOnce();
  });

  it("reveals chrome for otherwise unmapped remote activity", () => {
    handleTvWatchingRemoteEvent("right", false, port);

    expect(port.revealOverlay).toHaveBeenCalledOnce();
  });
});
