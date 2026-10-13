import { describe, expect, it } from "vitest";
import { approvalChannelEffect, newChannelName } from "../requests";

const intent = (description: string) => ({ description }) as Parameters<typeof newChannelName>[0]["intent"];

describe("newChannelName — the name the server gives a new channel", () => {
  it("prefers the proposal's channel name", () => {
    expect(newChannelName({ channelName: "Rainy afternoon serials", intent: intent("cozy mysteries") })).toBe(
      "Rainy afternoon serials",
    );
  });

  it("falls back to the request's description", () => {
    expect(newChannelName({ channelName: "  ", intent: intent("Overnight documentary block") })).toBe(
      "Overnight documentary block",
    );
  });

  it("falls back to the server's default when both are empty", () => {
    expect(newChannelName({ intent: intent("") })).toBe("Suggested channel");
  });

  // Same cut as binder.truncateLabel: 60 characters, back to the last space past the midpoint,
  // trailing punctuation trimmed, then an ellipsis.
  it("truncates a long name on a word boundary, exactly as the server does", () => {
    const long = "Saturday-morning style animation for the whole family, nothing past TV-Y7 please";
    expect(newChannelName({ intent: intent(long) })).toBe(
      "Saturday-morning style animation for the whole family…",
    );
  });

  it("keeps a name of exactly 60 characters whole", () => {
    const sixty = "a".repeat(60);
    expect(newChannelName({ intent: intent(sixty) })).toBe(sixty);
  });

  it("hard-cuts a long name with no usable space", () => {
    expect(newChannelName({ intent: intent("x".repeat(70)) })).toBe(`${"x".repeat(60)}…`);
  });
});

describe("approvalChannelEffect — create or update, never assumed", () => {
  const proposal = { channelName: "Weekend cartoon hour", intent: intent("cartoons") };

  it("creates a channel when none is bound to the job", () => {
    expect(approvalChannelEffect("job_1", proposal, [{ intentRef: "job_other", name: "Other" }])).toEqual({
      kind: "create",
      name: "Weekend cartoon hour",
    });
  });

  it("updates the channel already bound to the job, by its current name", () => {
    expect(
      approvalChannelEffect("job_1", proposal, [
        { intentRef: "job_other", name: "Other" },
        { intentRef: "job_1", name: "Cartoon Club" },
      ]),
    ).toEqual({ kind: "update", name: "Cartoon Club" });
  });

  it("never matches a hand-made channel (no intentRef) to a job", () => {
    expect(approvalChannelEffect("", proposal, [{ name: "Hand-made" }]).kind).toBe("create");
  });
});
