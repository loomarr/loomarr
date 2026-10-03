import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  getGetPlayoutStatusMockHandler,
  getMeMockHandler,
  getSettingsListMockHandler,
  getSystemBackupsListMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { SETTINGS_DESTINATIONS } from "@/settings/settings-destinations";
import type { SettingsBlock } from "@/settings/settings-page";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

// Issue #1524: a registry key with no row anywhere in the Settings UI is a silent regression — it
// is reachable only through Advanced settings' raw table, which a person browsing their actual
// task (Connections, AI, Playback, …) will never open. This guard fails the build the moment a
// non-internal registry key loses coverage, instead of waiting for someone to notice in Advanced.
//
// Source of truth: `docs/reference/settings.md`, generated from `internal/settings/declared.go` by
// `make config-docs` and already drift-checked against it (internal/settings/docs_test.go). Parsing
// THAT file — rather than hand-copying the key list here — is what keeps this guard honest: a key
// added to the registry without a `make config-docs` regen fails the Go drift test first, and a key
// added WITH a regen shows up here automatically.

interface DocRow {
  key: string;
  ownerTask: string;
  group: string;
}

// Mirrors `groupTitles` in internal/settings/docs.go — the registry Group → doc heading map. Only
// the heading text changes if that map changes, never the key list, so this is a small closed
// vocabulary (15 entries) rather than the hand-copied registry enumeration the guard exists to avoid.
const GROUP_HEADING_TO_WIRE: Record<string, string> = {
  "Connections — Media server": "connections.media_server",
  "Connections — Requester": "connections.requester",
  "Connections — Tunarr": "connections.tunarr",
  "Connections — TMDB": "connections.tmdb",
  General: "general",
  AI: "ai",
  "Channels & playback": "channels",
  Playout: "playout",
  "Filler / commercials": "filler",
  Backup: "backup",
  Notifications: "notifications",
  "Users & security": "users_security",
  "Single sign-on": "sso",
  Images: "images",
  Advanced: "advanced",
};

// Mirrors `ownerTitles` in internal/settings/docs.go crossed with `SETTINGS_DESTINATIONS`' `id` —
// only for the owners whose destination page renders registry keys through `SettingsPage`'s
// `blocks`. Owners absent here (Advanced settings, Background tasks, Diagnostics) route to pages
// that don't use `blocks` at all; see OWNER_TASK_SKIP below.
//
// The ten "Filler — …" entries are `FillerSettings` (web/apps/web/src/filler/filler-settings),
// one component rendered at every `/filler/settings/$section` destination — each render passes
// SettingsPage a single-block array filtered to that section, so each section still needs its own
// render to capture its block. Destination ids mirror `settings-destinations.ts`'s
// `filler.${section.id}` convention, not hand-picked.
const OWNER_TASK_TO_DESTINATION_ID: Record<string, string> = {
  Connections: "connections",
  AI: "ai",
  "Channel defaults": "defaults",
  Notifications: "notifications",
  Location: "location",
  "Sharing address": "sharing",
  "Access and devices": "access",
  Playback: "playback",
  Storage: "storage",
  Backups: "backup",
  "Filler — Clip folders": "filler.folders",
  "Filler — Automatic downloads": "filler.downloads",
  "Filler — Storage limits": "filler.storage",
  "Filler — Clip details": "filler.details",
  "Filler — Incoming history": "filler.incoming",
  "Filler — Break assembly": "filler.breaks",
  "Filler — Clip review": "filler.review",
  "Filler — Clip eligibility and sound": "filler.playback",
  "Filler — Processing limits": "filler.limits",
  "Filler — Processing tools": "filler.tools",
};

// These owner tasks route to pages with no `blocks` mechanism to check:
//  - "Advanced settings" → /settings/advanced, the raw AdvancedSettingsTable (deliberately NOT a
//    SettingsPage — see its own file comment). Every key routed here is, by construction, shown
//    only via Advanced/search.
//  - "Background tasks" → /settings/system/tasks, which edits job schedules through the Jobs API,
//    a different domain from the settings registry.
//  - "Diagnostics" → /settings/system/diagnostics, a logs/health surface with no settings fields.
const OWNER_TASK_SKIP = new Set(["Advanced settings", "Background tasks", "Diagnostics"]);

// Keys that deliberately have no row in `blocks` today, each with why. TODO(#1524): either give
// each of these a row, or keep it allowlisted on purpose — but never let one drop out silently.
const ALLOWLIST: Record<string, string> = {
  "llm.model":
    "Shown via the AI model picker (AiModelSettings on the AI page's footer), not a blocks[].keys row — declared.go documents this as the deliberate 'All Settings escape hatch'.",
  "filler.vision.provider":
    "Edited through AiModelSettings' vision picker (AI page footer), not a blocks[].keys row.",
  "filler.vision.model":
    "Edited through AiModelSettings' vision picker (AI page footer), not a blocks[].keys row.",
  "filler.vision.url":
    "Edited through AiModelSettings' vision picker (AI page footer), not a blocks[].keys row.",
  "filler.vision.api_key":
    "Edited through AiModelSettings' vision picker (AI page footer), not a blocks[].keys row.",
  "filler.home_country":
    "Edited through InstallationLocation on Access and devices (a children render-prop, not a blocks[].keys row).",
  "filler.home_market":
    "Edited through InstallationLocation on Access and devices (a children render-prop, not a blocks[].keys row).",
  "channel.reconcile_every":
    "No row today — Channel defaults' 'Schedule horizon' block only lists sched.window_hours for the channels group.",
  "playout.state_dir": "No row today — not listed in any Playback block's keys.",
  "playout.memory_reserve_mb": "No row today — not listed in any Playback block's keys.",
  "playout.encode_memory_mb": "No row today — not listed in any Playback block's keys.",
  "playout.gpu_cpu_millicores": "No row today — not listed in any Playback block's keys.",
  "playout.app_reserve_millicores": "No row today — not listed in any Playback block's keys.",
  "filler.language":
    "Edited through InstallationLocation's language picker on Access and devices, not a blocks[].keys row on its own 'Clip eligibility and sound' Filler section.",
  "filler.research.wikidata_enabled":
    "Edited through ClipDetailsSettings (the Filler 'Clip details' section's footer component), not a blocks[].keys row.",
  "filler.research.wikipedia_enabled":
    "Edited through ClipDetailsSettings (the Filler 'Clip details' section's footer component), not a blocks[].keys row.",
  "filler.research.archive_enabled":
    "Edited through ClipDetailsSettings (the Filler 'Clip details' section's footer component), not a blocks[].keys row.",
  "filler.research.loc_enabled":
    "Edited through ClipDetailsSettings (the Filler 'Clip details' section's footer component), not a blocks[].keys row.",
  "filler.research.web_provider":
    "Edited through ClipDetailsSettings' web-search sheet (the Filler 'Clip details' section's footer component), not a blocks[].keys row.",
  "filler.research.brave_api_key":
    "Edited through ClipDetailsSettings' web-search sheet (the Filler 'Clip details' section's footer component), not a blocks[].keys row.",
  "filler.research.searxng_url":
    "Edited through ClipDetailsSettings' web-search sheet (the Filler 'Clip details' section's footer component), not a blocks[].keys row.",
  "filler.research.monthly_limit":
    "Edited through ClipDetailsSettings (the Filler 'Clip details' section's footer component), not a blocks[].keys row.",
  "filler.acquisition.compilations":
    "No row today — the Filler 'Automatic downloads' section only lists filler.fetch.every and filler.fetch.max_per_run.",
  "filler.conditioning.normalize_loudness":
    "No row today — not listed in the Filler 'Clip review' section's keys.",
  "filler.structure_window_authority_path":
    "No row today — not listed in the Filler 'Clip review' section's keys.",
  "filler.structure_window_deployment_path":
    "No row today — not listed in the Filler 'Clip review' section's keys.",
  "filler.language_model":
    "No row today — not listed in the Filler 'Clip eligibility and sound' section's keys, and InstallationLocation only wires filler.language, not this one.",
};

const repoRoot = (): string => {
  let dir = dirname(fileURLToPath(import.meta.url));
  for (;;) {
    if (existsSync(join(dir, "go.mod"))) return dir;
    const parent = dirname(dir);
    if (parent === dir) throw new Error(`go.mod not found walking up from ${import.meta.url}`);
    dir = parent;
  }
};

const parseSettingsDocs = (): DocRow[] => {
  const path = join(repoRoot(), "docs", "reference", "settings.md");
  const md = readFileSync(path, "utf-8");
  const rows: DocRow[] = [];
  let heading = "";
  for (const line of md.split("\n")) {
    const headingMatch = /^##\s+(.+)$/.exec(line);
    if (headingMatch) {
      heading = headingMatch[1] as string;
      continue;
    }
    const rowMatch = /^\|\s*`([^`]+)`\s*\(`[^`]*`\)\s*\|\s*([^|]+?)\s*\|/.exec(line);
    if (!rowMatch) continue;
    const group = GROUP_HEADING_TO_WIRE[heading];
    if (group === undefined) {
      throw new Error(
        `settings.md has a table row under unrecognized heading "${heading}" — ` +
          "add it to GROUP_HEADING_TO_WIRE (mirroring internal/settings/docs.go groupTitles).",
      );
    }
    rows.push({ key: rowMatch[1] as string, ownerTask: (rowMatch[2] as string).trim(), group });
  }
  return rows;
};

const isCovered = (key: string, group: string, blocks: readonly SettingsBlock[]): boolean =>
  blocks.some((block) => block.group === group && (!block.keys || block.keys.includes(key)));

let capturedBlocks: SettingsBlock[] | undefined;

vi.mock("@/settings/settings-page", () => ({
  SettingsPage: (props: { blocks: SettingsBlock[] }) => {
    capturedBlocks = props.blocks;
    return null;
  },
}));

afterEach(() => window.sessionStorage.clear());

const blocksRenderedAt = async (path: string): Promise<SettingsBlock[]> => {
  capturedBlocks = undefined;
  server.use(
    getMeMockHandler(me()),
    getSettingsListMockHandler({ features: {}, settings: [] }),
    getGetPlayoutStatusMockHandler(),
    getSystemBackupsListMockHandler({
      backups: [],
      dir: "",
      retain: 7,
      schedule: "0 30 3 * * *",
      supported: true,
    }),
    ...appHandlers(),
  );
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [path] }),
  });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(capturedBlocks).toBeDefined());
  if (!capturedBlocks) throw new Error(`SettingsPage was never rendered at ${path}`);
  return capturedBlocks;
};

// One render per destination path, however many keys route to it.
const blocksCache = new Map<string, Promise<SettingsBlock[]>>();
const blocksForDestination = (destinationId: string): Promise<SettingsBlock[]> => {
  const destination = SETTINGS_DESTINATIONS.find((item) => item.id === destinationId);
  if (!destination) throw new Error(`No SETTINGS_DESTINATIONS entry for id "${destinationId}"`);
  const cached = blocksCache.get(destination.path);
  if (cached) return cached;
  const promise = blocksRenderedAt(destination.path);
  blocksCache.set(destination.path, promise);
  return promise;
};

describe("every non-internal registry key has a Settings UI row (#1524)", () => {
  const rows = parseSettingsDocs();

  it("parsed at least one row per in-scope group (sanity check on the parser itself)", () => {
    expect(rows.length).toBeGreaterThan(50);
  });

  it("has no stale allowlist entry for a key the registry no longer declares", () => {
    const known = new Set(rows.map((row) => row.key));
    const stale = Object.keys(ALLOWLIST).filter((key) => !known.has(key));
    expect(stale, "remove these from ALLOWLIST — the registry no longer declares them").toEqual([]);
  });

  for (const row of rows) {
    if (OWNER_TASK_SKIP.has(row.ownerTask)) continue;

    const allowlistReason = ALLOWLIST[row.key];
    if (allowlistReason !== undefined) {
      it(`${row.key} is explicitly allowlisted: ${allowlistReason}`, () => {
        expect(allowlistReason).toBeTruthy();
      });
      continue;
    }

    it(`${row.key} appears in a Settings page's blocks`, async () => {
      const destinationId = OWNER_TASK_TO_DESTINATION_ID[row.ownerTask];
      expect(
        destinationId,
        `"${row.key}"'s owner task "${row.ownerTask}" has no entry in OWNER_TASK_TO_DESTINATION_ID`,
      ).toBeDefined();
      const blocks = await blocksForDestination(destinationId as string);
      expect(
        isCovered(row.key, row.group, blocks),
        `"${row.key}" (group "${row.group}") has no row in any block on its owner's page — ` +
          "add it to a block's `keys`, or to the ALLOWLIST above with a reason and a TODO(#1524).",
      ).toBe(true);
    });
  }
});

// Proves the detector itself: a key with no covering block must fail, and one covered by a
// keys-less (whole-group) block, or an explicit keys[] match, must pass.
describe("isCovered", () => {
  it("flags a key whose group has no block at all", () => {
    expect(isCovered("new.fake_key", "playout", [{ group: "ai", title: "AI setup" }])).toBe(false);
  });

  it("flags a key excluded by an explicit keys[] subset on its own group's block", () => {
    const blocks: SettingsBlock[] = [{ group: "ai", title: "AI setup", keys: ["llm.provider"] }];
    expect(isCovered("llm.model", "ai", blocks)).toBe(false);
  });

  it("passes a key whose group's block has no keys[] filter (whole group shown)", () => {
    const blocks: SettingsBlock[] = [{ group: "connections.media_server", title: "Media server" }];
    expect(isCovered("library.url", "connections.media_server", blocks)).toBe(true);
  });

  it("passes a key explicitly named in its block's keys[]", () => {
    const blocks: SettingsBlock[] = [{ group: "ai", title: "AI setup", keys: ["llm.provider", "llm.url"] }];
    expect(isCovered("llm.url", "ai", blocks)).toBe(true);
  });
});
