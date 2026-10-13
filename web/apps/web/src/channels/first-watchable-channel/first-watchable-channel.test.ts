import { describe, expect, it } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { firstWatchableChannelId } from "./first-watchable-channel";

describe("firstWatchableChannelId", () => {
  it("picks the lowest-numbered playable channel", () => {
    const channels = [
      channel({ id: "ch-30", number: 30, inAppPlayable: true }),
      channel({ id: "ch-10", number: 10, inAppPlayable: true }),
      channel({ id: "ch-20", number: 20, inAppPlayable: true }),
    ];
    expect(firstWatchableChannelId(channels)).toBe("ch-10");
  });

  it("skips channels the server marked not in-app playable", () => {
    const channels = [
      channel({ id: "ch-5", number: 5, inAppPlayable: false }),
      channel({ id: "ch-10", number: 10, inAppPlayable: true }),
    ];
    expect(firstWatchableChannelId(channels)).toBe("ch-10");
  });

  it("is undefined with no channels — Watch's slot stays hidden (X2)", () => {
    expect(firstWatchableChannelId([])).toBeUndefined();
  });
});
