import { QueryClient } from "@tanstack/react-query";
import { createMemoryHistory, createRouter } from "@tanstack/react-router";
import { describe, expect, it } from "vitest";
import { routeTree } from "@/routeTree.gen";
import {
  cardDestinations,
  destinationById,
  destinationForOwner,
  SETTINGS_CARDS,
  SETTINGS_DESTINATIONS,
} from ".";

const PUBLIC_SETTING_OWNERS = [
  "settings.connections",
  "settings.ai",
  "settings.defaults",
  "settings.notifications",
  "settings.location",
  "settings.sharing",
  "settings.access",
  "settings.playback",
  "settings.storage",
  "settings.backup",
  "settings.tasks",
  "settings.diagnostics",
  "settings.advanced",
  "filler.folders",
  "filler.downloads",
  "filler.storage",
  "filler.details",
  "filler.incoming",
  "filler.breaks",
  "filler.review",
  "filler.playback",
  "filler.limits",
  "filler.tools",
] as const;

describe("settings destination catalog", () => {
  it("resolves every public registry owner exactly once", () => {
    const owners = SETTINGS_DESTINATIONS.flatMap((item) => (item.owner ? [item.owner] : []));
    expect(new Set(owners).size).toBe(owners.length);
    expect([...owners].sort()).toEqual([...PUBLIC_SETTING_OWNERS].sort());
    for (const owner of PUBLIC_SETTING_OWNERS) expect(destinationForOwner(owner)?.owner).toBe(owner);
  });

  it("lists each page once, and a part of a page as a pointer to it", () => {
    const pages = SETTINGS_DESTINATIONS.filter((item) => item.browse && !item.hostId);
    const paths = pages.map((item) => item.path);
    expect(paths.filter((path, index) => paths.indexOf(path) !== index)).toEqual([]);
    // Your location lives on the access page: the Channels card lists it "on" that page, which the
    // People and access card lists as itself. The sharing address is found by search only.
    const location = destinationById("location");
    expect(location?.card).toBe("channels");
    expect(destinationById(location?.hostId ?? "")?.path).toBe(location?.path);
    expect(destinationById("sharing")?.browse).toBe(false);
  });

  it("gives each of the seven cards a page, and puts Advanced under them", () => {
    expect(SETTINGS_CARDS.map((card) => card.title)).toEqual([
      "Connections",
      "AI",
      "Channels",
      "Filler",
      "People and access",
      "Notifications",
      "Server",
    ]);
    for (const card of SETTINGS_CARDS) expect(cardDestinations(card.id, true).length).toBeGreaterThan(0);
    expect(cardDestinations("advanced", true).map((item) => item.id)).toEqual(["advanced"]);
    expect(cardDestinations("server", true).map((item) => item.label)).toEqual([
      "Playback",
      "Artwork storage",
      "Backups",
      "Background tasks",
      "Diagnostics",
      "Database",
      "About Loomarr",
    ]);
  });

  it("leaves a member only the cards they can visit", () => {
    const visible = SETTINGS_CARDS.filter((card) => cardDestinations(card.id, false).length > 0);
    expect(visible.map((card) => card.id)).toEqual(["notifications"]);
    expect(cardDestinations("advanced", false)).toEqual([]);
  });

  it("points every destination at a registered route", () => {
    const router = createRouter({
      routeTree,
      history: createMemoryHistory(),
      context: { queryClient: new QueryClient() },
    });
    const registered = new Set(
      Object.keys(router.routesByPath).map((path) => path.replace(/\/$/, "") || "/"),
    );
    for (const item of SETTINGS_DESTINATIONS) {
      const path = item.path.match(
        /^\/filler\/settings\/(folders|downloads|storage|details|incoming|breaks|review|playback|limits|tools)$/,
      )
        ? "/filler/settings/$section"
        : item.path;
      expect(registered, `${item.id} → ${path}`).toContain(path);
    }
    expect(registered).not.toContain("/settings/general");
    expect(registered).not.toContain("/settings/security");
    expect(registered).not.toContain("/settings/all");
  });
});
