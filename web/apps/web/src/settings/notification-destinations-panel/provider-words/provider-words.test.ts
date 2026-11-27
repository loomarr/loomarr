import { describe, expect, it } from "vitest";
import { words } from "./provider-words";

describe("words", () => {
  it("reads a snake-case event as capitalised words", () => {
    expect(words("channel_degraded")).toBe("Channel Degraded");
    expect(words("transport_unavailable")).toBe("Transport Unavailable");
  });

  it("leaves a single word capitalised and an empty string empty", () => {
    expect(words("email")).toBe("Email");
    expect(words("")).toBe("");
  });
});
