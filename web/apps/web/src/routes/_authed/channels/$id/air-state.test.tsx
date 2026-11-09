import type { ChannelDTO } from "@loomarr/api";
import { describe, expect, it } from "vitest";
import { airStateOf, headerAirLine } from "./route";

// A minimal channel — only the fields airStateOf reads. Cast partial per this suite's
// convention (see -channel-advanced.test.tsx): the DTO is large and status/tunarrId/
// inAppPlayable are the only inputs to the air-state decision.
const channel = (over: Partial<ChannelDTO>): ChannelDTO =>
  ({ id: "ch1", name: "Saturday Cartoons", number: 42, ...over }) as ChannelDTO;

describe("airStateOf", () => {
  // The regression this fix exists for. Internal playout is the DEFAULT backend and never
  // receives a `tunarrId`; a live, in-app-playable channel is genuinely broadcasting and must
  // read "On air" — keying on `tunarrId` alone told every working internal-playout channel it
  // was "Not on air yet — connect Tunarr" at the first-channel "aha" moment.
  it("reads On air for a live internal-playout channel (no tunarrId, inAppPlayable)", () => {
    const state = airStateOf(channel({ status: "live", inAppPlayable: true }));
    expect(state.label).toBe("On air");
    expect(state.dot).toBe("live");
  });

  it("reads On air for a live Tunarr-backed channel (pushed projection)", () => {
    const state = airStateOf(channel({ status: "live", tunarrId: "tun-abc", inAppPlayable: false }));
    expect(state.label).toBe("On air");
  });

  // Live but not yet broadcasting on either backend — genuinely not on air. This is the case
  // the "Not on air yet" copy is for, and it must NOT fire for a working internal channel.
  it("reads Not on air yet for a live channel that is neither pushed nor in-app-playable", () => {
    const state = airStateOf(channel({ status: "live", inAppPlayable: false }));
    expect(state.label).toBe("Not on air yet");
    expect(state.dot).toBe("reconciling");
  });

  // #2728: "On air" sat beside "Programme information isn't available" and an empty guide row.
  // A live channel with nothing in now/next has no schedule, whatever its titles say.
  it("reads No schedule yet for a live channel with nothing in now/next", () => {
    const state = airStateOf(channel({ status: "live", inAppPlayable: true }), false);
    expect(state.label).toBe("No schedule yet");
    expect(state.dot).toBe("off");
    expect(state.buildSchedule).toBe(true);
  });

  // Before now/next has answered, the channel's own status stands: no flash of "No schedule".
  it("keeps On air while now/next hasn't answered", () => {
    const state = airStateOf(channel({ status: "live", inAppPlayable: true }), undefined);
    expect(state.label).toBe("On air");
    expect(state.buildSchedule).toBeFalsy();
  });

  it("reads Not on air yet while building", () => {
    expect(airStateOf(channel({ status: "building", inAppPlayable: false })).label).toBe("Not on air yet");
  });

  it("reads Off air when detached", () => {
    expect(airStateOf(channel({ status: "detached", inAppPlayable: false })).label).toBe("Off air");
  });

  it("reads Paused when paused", () => {
    expect(airStateOf(channel({ status: "paused", inAppPlayable: false })).label).toBe("Paused");
  });

  it("reads On air (catching up) when drifted", () => {
    expect(airStateOf(channel({ status: "drifted", inAppPlayable: true })).label).toBe(
      "On air (catching up)",
    );
  });
});

describe("headerAirLine", () => {
  const live = airStateOf(channel({ status: "live", inAppPlayable: true }));
  const now = { gap: false, startMs: 0, stopMs: 30 * 60_000, title: "Superman" };

  // #2748: the header names the airing the way Home, the Guide and the player do.
  it("names the airing as Series · Title, as everywhere else", () => {
    const airing = { kind: "program" as const, series: "Superman", title: "The Mad Scientist" };
    expect(headerAirLine(live, now, 10 * 60_000, airing).text).toBe(
      "On now: Superman · The Mad Scientist · 20m left",
    );
  });

  it("falls back to the now/next title while the timeline has not answered", () => {
    expect(headerAirLine(live, now, 10 * 60_000).text).toBe("On now: Superman · 20m left");
  });
});
