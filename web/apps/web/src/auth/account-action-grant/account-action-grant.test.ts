import { beforeEach, describe, expect, it } from "vitest";
import { clearAccountActionGrant, takeAccountActionGrantFromLocation } from "./account-action-grant";

describe("account action grant", () => {
  beforeEach(() => {
    clearAccountActionGrant();
    window.history.replaceState({}, "", "/");
    localStorage.clear();
    sessionStorage.clear();
  });

  it("moves a valid fragment grant into memory and cleans browser history", () => {
    const grant = "a".repeat(64);
    window.history.replaceState({ router: "kept" }, "", `/reset-password?source=email#grant=${grant}`);
    expect(takeAccountActionGrantFromLocation()).toBe(grant);
    expect(window.location.pathname + window.location.search + window.location.hash).toBe(
      "/reset-password?source=email",
    );
    expect(window.history.state).toEqual({ router: "kept" });
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
  });

  it("rejects malformed bearer material after cleaning the URL", () => {
    window.history.replaceState({}, "", "/join#grant=not-a-token");
    expect(takeAccountActionGrantFromLocation()).toBeUndefined();
    expect(window.location.pathname + window.location.hash).toBe("/join");
  });
});
