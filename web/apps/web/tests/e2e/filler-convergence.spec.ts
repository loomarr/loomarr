import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

const clip = {
  hash: "trusted-toy-spot",
  path: "trusted-toy-spot.mp4",
  name: "Trusted Toy Spot",
  kind: "commercial",
  durationMs: 30000,
  era: 1990,
  audience: "kids",
  category: "toys",
  isComposite: false,
  tagged: true,
  aiTagged: true,
  playCount: 0,
  playsCounted: true,
};

test("a fetched arrival becomes playable automatically after its checks complete", async ({ page }) => {
  await installMockBackend(page, { authed: true, role: "admin" });
  let fetched = false;
  let readyCommitted = false;
  const calls: string[] = [];
  const requestedPaths: string[] = [];

  await page.route("**/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const { pathname: path } = url;
    requestedPaths.push(path);
    const method = request.method();
    const reply = (body: unknown) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });

    if (path === "/v1/settings") {
      const setting = (key: string, value: string, kind: string = "string") => ({
        key,
        group: key.startsWith("filler.") ? "filler" : "advanced",
        kind,
        doc: key,
        advanced: false,
        secret: false,
        set: true,
        provenance: "db",
        value,
      });
      return reply({
        features: { filler: true },
        settings: [
          setting("setup.completed", "true", "bool"),
          setting("filler.dir", "/data/filler"),
          setting("filler.pod_max", "4", "int"),
          setting("filler.breaks_per_hour", "4", "int"),
          setting("filler.break_duration", "30s", "duration"),
        ],
      });
    }
    if (path === "/v1/filler/sources") {
      return reply({
        sources: [
          {
            id: "provider:archive",
            kind: "archive",
            target: "Archive.org",
            detail: "collections you added",
            count: 0,
            incoming: 0,
            configured: true,
            fetchable: false,
            enabled: true,
            effectiveEnabled: true,
            providerEnabled: true,
            switchable: false,
            removable: false,
            searchable: false,
            group: true,
            readiness: "ready",
            ready: true,
            locationSource: "missing",
            actions: ["configure", "disable"],
          },
          {
            id: "archive:trusted",
            uri: "trusted",
            kind: "archive",
            target: "Trusted Commercials",
            detail: "an enabled archive.org collection",
            count: fetched ? 1 : 0,
            incoming: fetched && !readyCommitted ? 1 : 0,
            configured: true,
            fetchable: true,
            enabled: true,
            effectiveEnabled: true,
            providerEnabled: true,
            switchable: true,
            removable: true,
            searchable: false,
            parentId: "provider:archive",
            readiness: "ready",
            ready: true,
            locationSource: "installation",
            actions: ["fetch", "disable", "remove", "edit_location"],
          },
        ],
        total: 1,
      });
    }
    if (path === "/v1/filler/sources/fetch" && method === "POST") {
      expect(url.searchParams.get("id")).toBe("archive:trusted");
      calls.push("bounded source fetch");
      fetched = true;
      return reply({ total: 1, added: 1, updated: 0, pruned: 0 });
    }
    if (path === "/v1/filler/readiness") {
      return reply({
        ready: readyCommitted,
        nextAction: readyCommitted ? "none" : "add_filler",
        fetch: { enabled: true, catalogClips: fetched ? 1 : 0 },
        pipeline: {
          runnable: fetched && !readyCommitted ? 1 : 0,
          scheduled: 0,
          inProgress: 0,
          needsDecision: 0,
          recoverable: 0,
          ready: readyCommitted ? 1 : 0,
          rejected: 0,
          dismissed: 0,
        },
        pool: {
          clips: readyCommitted ? 1 : 0,
          breakBody: readyCommitted ? 1 : 0,
          eligible: readyCommitted ? 1 : 0,
          untagged: 0,
          channels: readyCommitted
            ? [
                {
                  channelId: "ch-1",
                  name: "Saturday Mornings",
                  number: 1,
                  level: "exact",
                  total: 1,
                  durationMs: 30000,
                  categories: 1,
                  brands: 1,
                },
              ]
            : [],
        },
        acquisitions: fetched
          ? [
              {
                id: "acq-trusted",
                sourceId: "archive:trusted",
                trigger: "source",
                status: "success",
                requested: 1,
                fetched: 1,
                skipped: 0,
                failed: 0,
                empty: 0,
                startedAt: "2026-08-24T04:00:00Z",
                updatedAt: "2026-08-24T04:00:01Z",
                completedAt: "2026-08-24T04:00:01Z",
                outcome: {
                  enrolled: fetched ? 1 : 0,
                  preparing: fetched && !readyCommitted ? 1 : 0,
                  needsDecision: 0,
                  ready: readyCommitted ? 1 : 0,
                  rejected: 0,
                  dismissed: 0,
                },
              },
            ]
          : [],
      });
    }
    if (path === "/v1/filler/decisions/activity") {
      return reply({
        rows: readyCommitted
          ? [
              {
                id: "automatic-event",
                decisionId: "decision-2",
                clipHash: clip.hash,
                kind: "automatic_admit",
                createdAt: "2026-08-24T04:00:02Z",
              },
            ]
          : [],
        total: readyCommitted ? 1 : 0,
      });
    }
    if (path === "/v1/filler/decisions/diagnostics") return reply({ rows: [], total: 0 });
    if (path === "/v1/filler/watch") {
      return reply({
        sourcesOn: 1,
        sourcesReady: 1,
        sourcesTotal: 1,
        clips: fetched ? 1 : 0,
        held: fetched && !readyCommitted ? 1 : 0,
        health: "healthy",
      });
    }
    if (path === "/v1/filler/incoming") {
      const preparing = fetched && !readyCommitted;
      const status = {
        clipHash: clip.hash,
        name: clip.name,
        from: "Trusted Commercials",
        durationMs: clip.durationMs,
        statusLabel: preparing ? "Checking video" : "Ready",
        updatedAt: "2026-08-24T04:00:01Z",
        processing: { attempts: 1, stages: [] },
      };
      return reply({
        readyWindowSeconds: 86_400,
        preparing: { rows: preparing ? [status] : [], total: preparing ? 1 : 0 },
        needsHelp: { rows: [], total: 0 },
        recentlyReady: {
          rows: readyCommitted ? [status] : [],
          total: readyCommitted ? 1 : 0,
        },
      });
    }
    if (path === "/v1/filler") {
      return reply({
        clips: readyCommitted ? [clip] : [],
        total: readyCommitted ? 1 : 0,
      });
    }
    if (path === "/v1/channels/ch-1") {
      return reply({
        id: "ch-1",
        revision: 1,
        name: "Saturday Mornings",
        number: 1,
        inAppPlayable: true,
        status: "live",
        strategy: "shuffle",
        lineup: [],
        policy: { scope: { era: { from: 1990, to: 1999 } } },
        breakCount: readyCommitted ? 1 : 0,
        pendingCount: 0,
        programCount: 2,
        slotCount: readyCommitted ? 3 : 2,
      });
    }
    if (path === "/v1/channels/ch-1/filler/coverage") {
      return reply({
        level: "exact",
        total: readyCommitted ? 1 : 0,
        rungs: [{ level: "exact", clips: readyCommitted ? 1 : 0 }],
        criteria: [
          { criterion: "era", clips: readyCommitted ? 1 : 0 },
          { criterion: "audience", clips: readyCommitted ? 1 : 0 },
          { criterion: "category", clips: readyCommitted ? 1 : 0 },
          { criterion: "kind", clips: readyCommitted ? 1 : 0 },
          { criterion: "duration", clips: readyCommitted ? 1 : 0 },
          { criterion: "quality", clips: readyCommitted ? 1 : 0 },
        ],
      });
    }
    if (path === "/v1/channels/ch-1/pods/preview" && method === "POST") {
      calls.push("automatic break preview");
      return reply({
        coverage: {
          level: "exact",
          total: readyCommitted ? 1 : 0,
          rungs: [{ level: "exact", clips: readyCommitted ? 1 : 0 }],
          criteria: [],
        },
        entries: readyCommitted
          ? [
              {
                path: clip.path,
                tunarrProgramId: clip.hash,
                name: clip.name,
                kind: clip.kind,
                durationMs: 30000,
                isFallbackCard: false,
              },
            ]
          : [],
        totalMs: readyCommitted ? 30000 : 0,
        matchLevel: "exact",
      });
    }
    if (path === "/v1/taxonomy") {
      return reply({
        taxa: [],
        totalClips: readyCommitted ? 1 : 0,
        taggedClips: readyCommitted ? 1 : 0,
        unclassifiedClips: 0,
        axisCoverage: [],
      });
    }
    return route.fallback();
  });

  await page.goto("/filler/sources");
  await expect(page.getByText("Trusted Commercials")).toBeVisible();
  await expect(page.getByText(/every clip is checked before it can play/i)).toBeVisible();
  await expect(page.getByRole("switch", { name: /automatically file grounded clips/i })).toHaveCount(0);
  await page.getByRole("button", { name: /manage trusted commercials/i }).click();
  await page.getByRole("button", { name: "Look for new clips" }).click();
  await expect.poll(() => fetched).toBe(true);

  await page.goto("/filler/incoming");
  await expect(page.getByRole("heading", { name: "1 clip is getting ready" })).toBeVisible();
  await expect(page.getByText("Trusted Toy Spot", { exact: true })).toBeVisible();
  expect(requestedPaths).toContain("/v1/filler/incoming");

  await page.goto("/filler");
  await expect(page.getByRole("heading", { name: "Add filler to get started" })).toBeVisible();
  await expect(
    page.getByText("No playable filler is available yet. Add a source or drop in your own clips."),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: "Admission summary" })).toHaveCount(0);
  await expect(page.getByText("Needs judgment")).toHaveCount(0);
  await expect(page.getByText(/licen[cs]/i)).toHaveCount(0);
  expect(requestedPaths).not.toContain("/v1/filler/decisions/overview");
  const fillerNav = page.getByRole("navigation", { name: "Filler sections" });
  await expect(fillerNav.getByRole("link", { name: "Overview" })).toBeVisible();
  await expect(fillerNav.getByRole("link", { name: "Incoming" })).toBeVisible();
  await expect(fillerNav.getByRole("link", { name: /Library/ })).toBeVisible();
  await expect(fillerNav.getByRole("link", { name: "Manage" })).toBeVisible();
  await expect(fillerNav.getByRole("link", { name: "Sources" })).toBeVisible();

  await page.goto("/filler/manage");
  await expect(page.getByText("Nothing has happened yet.")).toBeVisible();

  await page.goto("/channels/ch-1/filler");
  await expect(page.getByRole("heading", { name: "Saved channel coverage" })).toBeVisible();
  await expect(page.getByText(/No clips match these choices yet\. Breaks use the bumper card/)).toBeVisible();
  await expect(page.getByRole("link", { name: "browse your filler library" })).toHaveAttribute(
    "href",
    "/filler/library",
  );
  await expect(page.getByText("Trusted Toy Spot", { exact: true })).toHaveCount(0);
  await expect(page.getByText("1 eligible commercial", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /apply filler/i })).toHaveCount(0);

  // This is a backend-state simulation, not an operator approval or UI publish action. It models
  // the automatic terminal Ready commit completing after fetch; the frontend only observes it.
  readyCommitted = true;

  await page.goto("/filler/manage");
  await expect(page.getByText("Added automatically")).toBeVisible();

  await page.goto("/filler/incoming");
  await expect(page.getByRole("heading", { name: "1 clip added to your Library" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Ready" })).toBeVisible();

  await page.goto("/channels/ch-1/filler");
  await expect(page.getByRole("heading", { name: "Saved channel coverage" })).toBeVisible();
  await expect(page.getByLabel("Pod segments")).toContainText("Trusted Toy Spot");
  await expect(page.getByRole("button", { name: /apply filler/i })).toHaveCount(0);
  expect(calls).toEqual(["bounded source fetch", "automatic break preview"]);

  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/filler/incoming");
  await page.evaluate(() => {
    document.body.style.zoom = "2";
  });
  const details = page.getByRole("button", { name: "View details for Trusted Toy Spot" });
  await details.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "Trusted Toy Spot" })).toBeVisible();
  await expect(page.getByText(/licen[cs]|admission|classification approval/i)).toHaveCount(0);
});
