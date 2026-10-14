import { describe, expect, it } from "vitest";
import { channelNavHighlight } from "./channel-nav-highlight";

describe("channelNavHighlight", () => {
  it("highlights Watch on a channel's watch route, whichever channel it is", () => {
    expect(channelNavHighlight("/channels/ch-1/watch")).toBe("watch");
    expect(channelNavHighlight("/channels/ch-42/watch")).toBe("watch");
  });

  it("highlights Guide on every channel-management route", () => {
    for (const section of ["info", "programming", "filler", "danger"]) {
      expect(channelNavHighlight(`/channels/ch-1/${section}`)).toBe("guide");
    }
  });

  it("is undefined everywhere else, including the bare channel route", () => {
    expect(channelNavHighlight("/guide")).toBeUndefined();
    expect(channelNavHighlight("/dashboard")).toBeUndefined();
    expect(channelNavHighlight("/channels/ch-1")).toBeUndefined();
  });
});
