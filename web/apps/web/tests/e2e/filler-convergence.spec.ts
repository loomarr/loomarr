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

test("a fetched arrival becomes playable only after terminal admission completes", async ({ page }) => {
  await installMockBackend(page, { authed: true, role: "admin" });
  let fetched = false;
  let terminalAdmissionApplied = false;
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
            id: "archive:trusted",
            uri: "trusted",
            kind: "archive",
            target: "Trusted Commercials",
            detail: "an enabled archive.org collection",
            count: fetched ? 1 : 0,
            configured: true,
            fetchable: true,
            enabled: true,
            switchable: true,
            removable: true,
            searchable: false,
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
        ready: terminalAdmissionApplied,
        nextAction: terminalAdmissionApplied ? "none" : "add_filler",
        fetch: { enabled: true, catalogClips: fetched ? 1 : 0 },
        pipeline: {
          runnable: fetched && !terminalAdmissionApplied ? 1 : 0,
          scheduled: 0,
          inProgress: 0,
          needsDecision: 0,
          recoverable: 0,
          admitted: terminalAdmissionApplied ? 1 : 0,
          rejected: 0,
          dismissed: 0,
        },
        pool: {
          clips: terminalAdmissionApplied ? 1 : 0,
          commercials: terminalAdmissionApplied ? 1 : 0,
          eligible: terminalAdmissionApplied ? 1 : 0,
          untagged: 0,
          channels: terminalAdmissionApplied
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
                  preparing: fetched && !terminalAdmissionApplied ? 1 : 0,
                  needsDecision: 0,
                  admitted: terminalAdmissionApplied ? 1 : 0,
                  rejected: 0,
                  dismissed: 0,
                },
              },
            ]
          : [],
      });
    }
    if (path === "/v1/filler/decisions/overview") {
      return reply({
        healthy: false,
        nextAction: "review_decisions",
        actionCount: 1,
        counts: {
          admitted: terminalAdmissionApplied ? 1 : 0,
          rejected: 0,
          reviews: 1,
          unresolvedReviews: 1,
          operational: 0,
          retryable: 0,
        },
      });
    }
    if (path === "/v1/filler/attention") {
      const rows = [
        {
          id: "review-1",
          clipHash: "ambiguous-spot",
          taskKind: "identity_role",
          applicationMode: "shadow",
          allowedActions: ["admit", "reject", "correct", "abandon"],
          question: "Is this a toy commercial?",
          reasonCodes: ["conflict_product"],
          evidenceRefs: ["closing-frame"],
          conflicts: [
            {
              claim: "product",
              values: ["toy", "programme excerpt"],
              evidenceRefs: ["closing-frame"],
              resolved: false,
            },
          ],
          createdAt: "2026-08-24T03:59:00Z",
        },
      ];
      return reply({ rows, total: rows.length });
    }
    if (path === "/v1/filler/attention/review-1/actions" && method === "POST") {
      const body = request.postDataJSON();
      expect(body).toMatchObject({ kind: "abandon", reason: "skip for now" });
      calls.push("review skipped without verdict");
      return reply({ id: body.actionId });
    }
    if (path === "/v1/filler/decisions/activity") {
      return reply({
        rows: fetched
          ? [
              {
                id: "shadow-event",
                decisionId: "decision-1",
                clipHash: clip.hash,
                kind: "automatic_admit",
                applicationMode: "shadow",
                createdAt: "2026-08-24T04:00:01Z",
              },
              ...(terminalAdmissionApplied
                ? [
                    {
                      id: "applied-event",
                      decisionId: "decision-2",
                      clipHash: clip.hash,
                      kind: "automatic_admit",
                      applicationMode: "applied",
                      createdAt: "2026-08-24T04:00:02Z",
                    },
                  ]
                : []),
            ]
          : [],
        total: fetched ? (terminalAdmissionApplied ? 2 : 1) : 0,
      });
    }
    if (path === "/v1/filler/decisions/diagnostics") return reply({ rows: [], total: 0 });
    if (path === "/v1/filler/watch") {
      return reply({
        sourcesOn: 1,
        sourcesTotal: 1,
        clips: fetched ? 1 : 0,
        held: fetched && !terminalAdmissionApplied ? 1 : 0,
        health: "healthy",
      });
    }
    if (path === "/v1/filler/incoming") {
      const incoming = fetched && !terminalAdmissionApplied;
      return reply({
        clips: incoming
          ? [
              {
                ...clip,
                from: "Trusted Commercials",
                reason: "Record the admission check",
                pipeline: {
                  lifecycle: "runnable",
                  progress: 0,
                  stage: "admission",
                  stages: [],
                  status: "pending",
                  updatedAt: "2026-08-24T04:00:01Z",
                },
              },
            ]
          : [],
        clipsTotal: incoming ? 1 : 0,
        decisionsTotal: 0,
        overview: {
          runnable: incoming ? 1 : 0,
          scheduled: 0,
          inProgress: 0,
          needsDecision: 0,
          recoverable: 0,
          admitted: terminalAdmissionApplied ? 1 : 0,
          rejected: 0,
          dismissed: 0,
        },
        reels: [],
        reelsTotal: 0,
        rejected: [],
        rejectedTotal: 0,
        stageOrder: ["admission"],
        total: 0,
      });
    }
    if (path === "/v1/filler") {
      return reply({
        clips: terminalAdmissionApplied ? [clip] : [],
        total: terminalAdmissionApplied ? 1 : 0,
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
        breakCount: terminalAdmissionApplied ? 1 : 0,
        pendingCount: 0,
        programCount: 2,
        slotCount: terminalAdmissionApplied ? 3 : 2,
      });
    }
    if (path === "/v1/channels/ch-1/filler/coverage") {
      return reply({
        level: "exact",
        total: terminalAdmissionApplied ? 1 : 0,
        rungs: [{ level: "exact", clips: terminalAdmissionApplied ? 1 : 0 }],
        criteria: [
          { criterion: "era", clips: terminalAdmissionApplied ? 1 : 0 },
          { criterion: "audience", clips: terminalAdmissionApplied ? 1 : 0 },
          { criterion: "category", clips: terminalAdmissionApplied ? 1 : 0 },
          { criterion: "kind", clips: terminalAdmissionApplied ? 1 : 0 },
          { criterion: "duration", clips: terminalAdmissionApplied ? 1 : 0 },
          { criterion: "quality", clips: terminalAdmissionApplied ? 1 : 0 },
        ],
      });
    }
    if (path === "/v1/channels/ch-1/pods/preview" && method === "POST") {
      calls.push("automatic break preview");
      return reply({
        coverage: {
          level: "exact",
          total: terminalAdmissionApplied ? 1 : 0,
          rungs: [{ level: "exact", clips: terminalAdmissionApplied ? 1 : 0 }],
          criteria: [],
        },
        entries: terminalAdmissionApplied
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
        totalMs: terminalAdmissionApplied ? 30000 : 0,
        matchLevel: "exact",
      });
    }
    if (path === "/v1/taxonomy") {
      return reply({
        taxa: [],
        totalClips: terminalAdmissionApplied ? 1 : 0,
        taggedClips: terminalAdmissionApplied ? 1 : 0,
        unclassifiedClips: 0,
        axisCoverage: [],
      });
    }
    return route.fallback();
  });

  await page.goto("/filler/attention");
  await expect(page.getByRole("heading", { name: "Is this a toy commercial?" })).toBeVisible();
  await page.getByRole("button", { name: "Skip for now" }).click();
  await expect(page.getByText("You're caught up for now")).toBeVisible();
  expect(calls).toContain("review skipped without verdict");

  await page.goto("/filler/sources");
  await expect(page.getByText("Trusted Commercials")).toBeVisible();
  await expect(
    page.getByText(/arrivals remain in Incoming until their safety, rights, playback/i),
  ).toBeVisible();
  await expect(page.getByRole("switch", { name: /automatically file grounded clips/i })).toHaveCount(0);
  await page.getByRole("button", { name: /fetch now from trusted commercials/i }).click();
  await expect.poll(() => fetched).toBe(true);

  const legacyIncomingRequests = requestedPaths.filter((path) => path === "/v1/filler/incoming").length;
  await page.goto("/filler/incoming");
  await expect(page.getByRole("heading", { name: "Is this a toy commercial?" })).toBeVisible();
  await expect(page.getByText("Trusted Toy Spot", { exact: true })).toHaveCount(0);
  expect(requestedPaths.filter((path) => path === "/v1/filler/incoming")).toHaveLength(legacyIncomingRequests);

  await page.goto("/filler");
  await expect(page.getByText("A few clips need your judgment")).toBeVisible();
  await expect(
    page.getByText("1 clip could not be classified safely without a person.", { exact: true }),
  ).toBeVisible();
  const fillerNav = page.getByRole("navigation", { name: "Filler sections" });
  await expect(fillerNav.getByRole("link", { name: "Overview" })).toBeVisible();
  await expect(fillerNav.getByRole("link", { name: "Incoming" })).toBeVisible();
  await expect(fillerNav.getByRole("link", { name: /Library/ })).toBeVisible();
  await expect(fillerNav.getByRole("link", { name: "Manage" })).toBeVisible();
  await expect(fillerNav.getByRole("link", { name: "Sources" })).toBeVisible();

  await page.goto("/filler/manage");
  await expect(page.getByText("Would admit (shadow)")).toBeVisible();

  await page.goto("/channels/ch-1/filler");
  await expect(page.getByRole("heading", { name: "Saved channel coverage" })).toBeVisible();
  await expect(page.getByText("No clips match this selection yet.")).toBeVisible();
  await expect(page.getByText("Trusted Toy Spot", { exact: true })).toHaveCount(0);
  await expect(page.getByText("1 eligible commercial", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /apply filler/i })).toHaveCount(0);

  await page.goto("/filler/attention");
  await expect(page.getByRole("heading", { name: "Is this a toy commercial?" })).toBeVisible();

  // This is a backend-state simulation, not an operator approval or UI publish action. It models
  // certified automatic terminal admission completing after fetch; the frontend only observes it.
  terminalAdmissionApplied = true;

  await page.goto("/filler/manage");
  await expect(page.getByText("Would admit (shadow)")).toBeVisible();
  await expect(page.getByText("Admitted automatically")).toBeVisible();

  await page.goto("/channels/ch-1/filler");
  await expect(page.getByRole("heading", { name: "Saved channel coverage" })).toBeVisible();
  await expect(page.getByLabel("Pod segments")).toContainText("Trusted Toy Spot");
  await expect(page.getByRole("button", { name: /apply filler/i })).toHaveCount(0);
  expect(calls).toEqual([
    "review skipped without verdict",
    "bounded source fetch",
    "automatic break preview",
  ]);
});
