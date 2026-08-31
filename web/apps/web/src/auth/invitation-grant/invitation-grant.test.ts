import { beforeEach, describe, expect, it } from "vitest";
import { takeInvitationGrantFromLocation } from "./invitation-grant";

describe("takeInvitationGrantFromLocation", () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
    window.history.replaceState({ router: "kept" }, "", "/join?source=qr");
  });

  it("moves a valid fragment bearer to memory and immediately replaces token-bearing history", () => {
    const grant = "d".repeat(64);
    window.history.replaceState({ router: "kept" }, "", `/join?source=qr#grant=${grant}`);

    expect(takeInvitationGrantFromLocation()).toBe(grant);
    expect(window.location.pathname + window.location.search + window.location.hash).toBe("/join?source=qr");
    expect(window.history.state).toEqual({ router: "kept" });
    expect(localStorage).toHaveLength(0);
    expect(sessionStorage).toHaveLength(0);
  });

  it("returns no credential for malformed fragments while still removing them from history", () => {
    window.history.replaceState({}, "", "/join#grant=not-a-token");

    expect(takeInvitationGrantFromLocation()).toBeUndefined();
    expect(window.location.hash).toBe("");
  });
});
