import { describe, expect, it } from "vitest";
import { phoneBarOverflow, phoneBarPrimary, phoneDestinationMatches } from "./destinations";

describe("phoneBarPrimary", () => {
  it("is Home, Watch, Guide, Requests for an admin when a channel exists", () => {
    const withChannel = phoneBarPrimary(true, "chan-1").map((d) => d.key);
    expect(withChannel).toEqual(["home", "watch", "guide", "requests"]);
  });

  it("is Home, Watch, Guide, Library for a member, who browses the titles there (#4476)", () => {
    const member = phoneBarPrimary(false, "chan-1");
    expect(member.map((d) => d.label)).toEqual(["Home", "Watch", "Guide", "Library"]);
    expect(member.at(-1)).toMatchObject({ to: "/library" });
  });

  it("drops Watch's slot entirely rather than leaving a gap or greying it (X2)", () => {
    expect(phoneBarPrimary(true, undefined).map((d) => d.key)).toEqual(["home", "guide", "requests"]);
    expect(phoneBarPrimary(false, undefined).map((d) => d.key)).toEqual(["home", "guide", "library"]);
  });

  it("points Watch at the given channel id", () => {
    const [, watch] = phoneBarPrimary(true, "chan-42");
    expect(watch).toMatchObject({ params: { id: "chan-42" }, to: "/watch/$id" });
  });

  it("carries Requests' pending count when positive, and nothing at zero", () => {
    const zero = phoneBarPrimary(true, "chan-1", { "/requests": 0 }).find((d) => d.key === "requests");
    const some = phoneBarPrimary(true, "chan-1", { "/requests": 3 }).find((d) => d.key === "requests");
    expect(zero?.badgeCount).toBeUndefined();
    expect(some?.badgeCount).toBe(3);
  });
});

describe("phoneBarOverflow", () => {
  it("gives an admin exactly Library, Filler, People, Settings, Help", () => {
    expect(phoneBarOverflow(true).map((d) => d.key)).toEqual([
      "library",
      "filler",
      "people",
      "settings",
      "help",
    ]);
  });

  it("gives a member exactly Requests, Notifications, Help — never the admin-only surfaces", () => {
    expect(phoneBarOverflow(false).map((d) => d.key)).toEqual(["requests", "notifications", "help"]);
  });

  it("carries a member's Requests count into More", () => {
    const requests = phoneBarOverflow(false, { "/requests": 2 }).find((d) => d.key === "requests");
    expect(requests?.badgeCount).toBe(2);
  });
});

describe("phoneDestinationMatches", () => {
  it("matches any /watch/:id URL as the Watch destination", () => {
    expect(phoneDestinationMatches("/watch/chan-9", { to: "/watch/$id" })).toBe(true);
    expect(phoneDestinationMatches("/channels/chan-9", { to: "/watch/$id" })).toBe(false);
  });

  it("matches a plain destination exactly or as a nested route", () => {
    expect(phoneDestinationMatches("/settings", { to: "/settings" })).toBe(true);
    expect(phoneDestinationMatches("/settings/system/about", { to: "/settings" })).toBe(true);
    expect(phoneDestinationMatches("/settings/notifications", { to: "/settings/notifications" })).toBe(true);
    expect(phoneDestinationMatches("/guide", { to: "/settings" })).toBe(false);
  });
});
