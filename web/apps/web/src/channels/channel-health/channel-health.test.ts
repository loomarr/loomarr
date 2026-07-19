import type { ChannelDTO } from "@loomarr/api";
import { describe, expect, it } from "vitest";
import { channelHealth, channelOnAir } from "./channel-health";

const channel = (over: Partial<ChannelDTO> = {}): ChannelDTO => ({
  id: "ch-1",
  name: "Saturday Cartoons",
  number: 42,
  status: "live",
  strategy: "shuffle",
  programCount: 10,
  slotCount: 10,
  policy: {},
  ...over,
});

describe("channelHealth", () => {
  it("maps the lifecycle states an operator must act on", () => {
    expect(channelHealth(channel({ status: "building" }))).toBe("creating");
    expect(channelHealth(channel({ status: "drifted" }))).toBe("drift");
    // Tunarr lost a channel Loomarr thinks it manages — the one state needing action.
    expect(channelHealth(channel({ status: "detached" }))).toBe("error");
  });

  it("is healthy only when every slot has a real program", () => {
    expect(channelHealth(channel({ programCount: 10, slotCount: 10 }))).toBe("healthy");
  });

  it("reports pending-slots while acquisitions are still landing", () => {
    // The channel IS airing (Tunarr plays flex, never dead air) but it is not yet what
    // was asked for. Calling this healthy would hide the backfill the operator awaits.
    expect(channelHealth(channel({ programCount: 3, slotCount: 10 }))).toBe("pending-slots");
  });
});

describe("channelOnAir", () => {
  it("separates 'is it broadcasting' from 'is it correct'", () => {
    expect(channelOnAir(channel({ status: "live" }))).toBe("live");
    // Drifted still broadcasts — it just no longer matches Loomarr's intent.
    expect(channelOnAir(channel({ status: "drifted" }))).toBe("live");
    expect(channelOnAir(channel({ status: "building" }))).toBe("reconciling");
    expect(channelOnAir(channel({ status: "detached" }))).toBe("off");
  });
});
