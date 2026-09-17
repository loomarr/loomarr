import { QueryClient } from "@tanstack/react-query";
import { createMemoryHistory, createRouter } from "@tanstack/react-router";
import { describe, expect, it } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { destinationForOwner, SETTINGS_DESTINATIONS } from ".";

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
        /^\/filler\/settings\/(folders|downloads|storage|incoming|breaks|review|playback|limits|tools)$/,
      )
        ? "/filler/settings/$section"
        : item.path;
      expect(registered, `${item.id} → ${path}`).toContain(path);
    }
  });
});
