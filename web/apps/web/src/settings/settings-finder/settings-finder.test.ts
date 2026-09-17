import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import { describe, expect, it } from "vitest";
import { setting } from "@/test/fixtures/settings";
import { findSettings } from ".";

const entries: SettingEntry[] = [
  setting({
    key: "library.url",
    label: "Media server address",
    envVar: "LIBRARY_URL",
    owner: "settings.connections",
    group: "connections.media_server",
    doc: "The address of your Emby or Jellyfin server.",
  }),
  setting({
    key: "filler.storage.library_budget_gb",
    label: "Filler storage allowance",
    envVar: "FILLER_STORAGE_LIBRARY_BUDGET_GB",
    owner: "filler.storage",
    group: "filler",
    kind: "int",
    doc: "Choose how much space Loomarr may use for filler, or leave it automatic.",
  }),
  setting({
    key: "session.ttl",
    label: "Sign-in lifetime",
    envVar: "SESSION_TTL",
    owner: "settings.access",
    group: "users_security",
    kind: "duration",
    doc: "How long you stay signed in.",
  }),
  setting({
    key: "notifications.browser.enabled",
    label: "Browser notifications",
    envVar: "NOTIFICATIONS_BROWSER_ENABLED",
    owner: "settings.notifications",
    group: "notifications",
    doc: "Notify this person in their browser.",
  }),
];

const first = (query: string, isAdmin = true) => findSettings({ query, entries, isAdmin })[0];

describe("settings finder", () => {
  it("understands household language and common service names", () => {
    expect(first("where I live")?.destination.id).toBe("location");
    expect(first("Jellyfin")?.destination.id).toBe("connections");
    expect(first("stop clips filling disk")?.destination.id).toBe("filler.storage");
  });

  it("finds exact keys and environment variables without searching values", () => {
    expect(first("filler.storage.library_budget_gb")?.setting?.key).toBe("filler.storage.library_budget_gb");
    expect(first("LIBRARY_URL")?.setting?.key).toBe("library.url");
    expect(findSettings({ query: "http://private.example", entries, isAdmin: true })).toEqual([]);
  });

  it("finds exact Filler tasks and records their Settings provenance", () => {
    const result = first("Incoming history");
    expect(result?.destination.path).toBe("/filler/settings/incoming");
    expect(result?.groupLabel).toBe("Filler");
  });

  it("filters destinations and settings by role", () => {
    const memberResults = findSettings({ query: "notifications", entries, isAdmin: false });
    expect(memberResults.map((result) => result.destination.id)).toEqual(["notifications"]);
    expect(findSettings({ query: "session", entries, isAdmin: false })).toEqual([]);
  });

  it("returns no results for empty or unmatched input", () => {
    expect(findSettings({ query: "   ", entries, isAdmin: true })).toEqual([]);
    expect(findSettings({ query: "something Loomarr has never heard of", entries, isAdmin: true })).toEqual(
      [],
    );
  });
});
