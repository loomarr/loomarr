import { describe, expect, it } from "vitest";
import { phoneBarOverflow, phoneBarPrimary, phoneDestinationMatches } from "./phone-bottom-bar.destinations";

describe("phoneBarPrimary", () => {
  it("is Home, Watch, Guide, Requests when a channel exists — identical for admin and member", () => {
    const withChannel = phoneBarPrimary("chan-1").map((d) => d.key);
    expect(withChannel).toEqual(["home", "watch", "guide", "requests"]);
  });

  it("drops Watch's slot entirely rather than leaving a gap or greying it (X2)", () => {
    const noChannel = phoneBarPrimary(undefined).map((d) => d.key);
    expect(noChannel).toEqual(["home", "guide", "requests"]);
  });

  it("points Watch at the given channel id", () => {
    const [, watch] = phoneBarPrimary("chan-42");
    expect(watch).toMatchObject({ params: { id: "chan-42" }, to: "/channels/$id/watch" });
  });

  it("carries Requests' pending count when positive, and nothing at zero", () => {
    const zero = phoneBarPrimary("chan-1", { "/requests": 0 }).find((d) => d.key === "requests");
    const some = phoneBarPrimary("chan-1", { "/requests": 3 }).find((d) => d.key === "requests");
    expect(zero?.badgeCount).toBeUndefined();
    expect(some?.badgeCount).toBe(3);
  });
});

describe("phoneBarOverflow", () => {
  it("gives an admin exactly Filler, People, Settings, Help", () => {
    expect(phoneBarOverflow(true).map((d) => d.key)).toEqual(["filler", "people", "settings", "help"]);
  });

  it("gives a member exactly Notifications, Help — never the admin-only surfaces", () => {
    expect(phoneBarOverflow(false).map((d) => d.key)).toEqual(["notifications", "help"]);
  });
});

describe("phoneDestinationMatches", () => {
  it("matches any /channels/:id/watch URL as the Watch destination", () => {
    expect(phoneDestinationMatches("/channels/chan-9/watch", { to: "/channels/$id/watch" })).toBe(true);
    expect(phoneDestinationMatches("/channels/chan-9", { to: "/channels/$id/watch" })).toBe(false);
  });

  it("matches a plain destination exactly or as a nested route", () => {
    expect(phoneDestinationMatches("/settings", { to: "/settings" })).toBe(true);
    expect(phoneDestinationMatches("/settings/system/about", { to: "/settings" })).toBe(true);
    expect(phoneDestinationMatches("/settings/notifications", { to: "/settings/notifications" })).toBe(true);
    expect(phoneDestinationMatches("/guide", { to: "/settings" })).toBe(false);
  });
});
