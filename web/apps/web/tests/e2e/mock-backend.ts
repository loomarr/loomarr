import type { Page, Route } from "@playwright/test";
import { outlook } from "../../src/test/fixtures/outlook";

// A stateful stand-in for /v1, installed with Playwright route interception. It is
// deliberately NOT a second API implementation: it answers only what the wizard calls,
// and it MUTATES as the operator acts — signing in flips `me`, wiring a connection turns
// its check green — because a first-run flow that can't progress proves nothing. The
// shapes come from api/openapi.yaml; if the contract moves, these break loudly.
const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };

const CANDIDATES = [
  { id: "u-ada", name: "Ada", isAdmin: true, disabled: false, imported: false },
  { id: "u-bo", name: "Bo", isAdmin: false, disabled: false, imported: false },
];

interface MockOptions {
  // Start signed out (the true first run) or already authenticated.
  authed?: boolean;
  // Whether the install has an owning admin (§7 GET /v1/setup/state). Defaults to
  // TRUE — most specs sign in, and an unclaimed install redirects every route to the
  // wizard. Set false to exercise the genuine first run, where /login must bounce to
  // /wizard because no credential could work yet.
  bootstrapped?: boolean;
  // Who is signed in. The approve-flow smoke runs the SAME screens as both, because
  // §7's gate is a role check and a member must be refused (§19).
  role?: "admin" | "member";
  // Seed a proposal already awaiting approval, as if a member submitted it earlier.
  // The gate's interesting case is an admin acting on SOMEONE ELSE'S work — that is the
  // only path from a proposal to spent resources (§7).
  pendingProposal?: boolean;
  // Make each submitted builder request finish as an authoritative failed Journey. This is
  // opt-in because the normal proposal/approval smoke deliberately exercises a different
  // terminal path.
  failedProposalJourney?: boolean;
  // Return a persisted, reviewable Journey for fresh-start route recovery coverage.
  proposalJourney?: boolean;
  // Which setup/status checks are green before the operator does anything. The two
  // REQUIRED ones default green so the flow can reach the wiring steps.
  checks?: Record<string, boolean>;
  // Expose the configured Filler shell for page-level navigation and accessibility
  // contracts. Most wizard flows intentionally leave this capability out of scope.
  fillerEnabled?: boolean;
}

interface MockBackend {
  // What the run recorded — the smoke asserts against these rather than re-reading UI.
  readonly state: {
    authed: boolean;
    checks: Record<string, boolean>;
    imported: string[];
    edits: Record<string, string>;
    // Titles the approval gate actually enqueued. The smoke asserts on THIS rather than
    // on the UI, because "the button looked like it worked" is exactly the failure a
    // gate test exists to catch.
    enqueued: string[];
    proposals: Array<{ id: string; status: string }>;
    // Exact bodies sent to the real proposal-submission endpoint. Recovery specs use this
    // as their outcome proof: an edit/retry must submit every intent constraint again.
    proposalJobRequests: Record<string, unknown>[];
    // Same-Job revisions remain distinct from fresh submissions so a browser test
    // can prove that editing a landed brief did not restart the journey.
    proposalRevisionRequests: Array<{ jobId: string; intent: Record<string, unknown> }>;
    // Mutation telemetry is deliberately request-level: no UI assertion can prove that a
    // failed Journey did not try a forbidden channel write before rendering its recovery.
    channelCreationRequests: Record<string, unknown>[];
    approvalRequests: string[];
    approvalEdits: Record<string, unknown>[];
    fillerFetches: string[];
    fillerSourcePatches: Array<{ id: string; body: Record<string, unknown> }>;
    fillerSourcePolicy: { mode: "defaults" | "custom" | "never"; everySeconds: number; maxPerCheck: number };
    fillerSourceItems: Array<{ sourceId: string; remoteId: string; url: string }>;
    fillerAcquisitionStatus: "queued" | "running" | "success" | "error";
  };
}

const json = (route: Route, body: unknown, status = 200) =>
  route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });

const installMockBackend = async (page: Page, opts: MockOptions = {}): Promise<MockBackend> => {
  const state = {
    authed: opts.authed ?? false,
    bootstrapped: opts.bootstrapped ?? true,
    checks: { media_server: true, tunarr: true, ...(opts.checks ?? {}) } as Record<string, boolean>,
    imported: [] as string[],
    edits: {} as Record<string, string>,
    enqueued: [] as string[],
    proposalJobRequests: [] as Record<string, unknown>[],
    proposalRevisionRequests: [] as Array<{ jobId: string; intent: Record<string, unknown> }>,
    proposalRevisions: {} as Record<string, { intent: Record<string, unknown>; pendingJourneyReads: number }>,
    channelCreationRequests: [] as Record<string, unknown>[],
    approvalRequests: [] as string[],
    approvalEdits: [] as Record<string, unknown>[],
    fillerFetches: [] as string[],
    fillerSourcePatches: [] as Array<{ id: string; body: Record<string, unknown> }>,
    fillerSourcePolicy: {
      mode: "defaults" as "defaults" | "custom" | "never",
      everySeconds: 21600,
      maxPerCheck: 10,
    },
    fillerSourceItems: [] as Array<{ sourceId: string; remoteId: string; url: string }>,
    fillerAcquisitionStatus: "queued" as "queued" | "running" | "success" | "error",
    proposals: (opts.pendingProposal ? [{ id: "prop-1", status: "submitted" }] : []) as Array<{
      id: string;
      status: string;
    }>,
    role: opts.role ?? "admin",
  };

  // The SSE stream: the app opens it for the session's lifetime. Left hanging open and
  // silent — this suite is about the setup flow, and a closed stream would have the app
  // reconnect-looping under the snapshots.
  await page.route("**/v1/events**", (route) => route.abort());

  await page.route("**/v1/**", async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname;
    const method = req.method();
    const body = () => {
      try {
        return JSON.parse(req.postData() ?? "{}") as Record<string, unknown>;
      } catch {
        return {};
      }
    };

    // --- identity ---------------------------------------------------------------
    if (path === "/v1/auth/me") {
      const me = { ...ADMIN, role: state.role, autoApprove: state.role === "admin" };
      return state.authed ? json(route, me) : json(route, { title: "Unauthorized" }, 401);
    }
    if (path === "/v1/auth/login" && method === "POST") {
      state.authed = true;
      return json(route, ADMIN);
    }
    if (path === "/v1/system/version") {
      return json(route, { version: "v0.9.3", ready: true, dirty: false });
    }
    if (path === "/v1/diagnostics/health") {
      return json(route, {
        generationId: "e2e-generation",
        generation: 1,
        version: "v0.9.3",
        processStartedAt: 1_780_000_000_000,
        generationStartedAt: 1_780_000_001_000,
        updatedAt: 1_780_000_002_000,
        nextRefreshAt: 1_780_000_062_000,
        state: "healthy",
        checks: [],
      });
    }
    if (path === "/v1/diagnostics/startup-reports") {
      const current = {
        id: "e2e-generation",
        generation: 1,
        version: "v0.9.3",
        processStartedAt: 1_780_000_000_000,
        generationStartedAt: 1_780_000_001_000,
        generationEndedAt: 1_780_000_002_000,
        durationMillis: 1_000,
        state: "ready",
        checks: [],
      };
      return json(route, { current, items: [current] });
    }
    // Unauthenticated (§7): the router guards read this BEFORE any session exists, to
    // tell an unclaimed install from a merely signed-out one.
    if (path === "/v1/setup/state") {
      return json(route, { bootstrapped: state.bootstrapped });
    }
    if (path === "/v1/setup/bootstrap" && method === "POST") {
      state.bootstrapped = true;
      return json(route, { id: ADMIN.id, name: ADMIN.name, role: ADMIN.role });
    }

    // --- the checklist the whole wizard is derived from --------------------------
    if (path === "/v1/setup/status") {
      const checks = Object.entries(state.checks).map(([name, ok]) => ({
        name,
        ok,
        hint: ok ? undefined : `${name} is not configured yet.`,
      }));
      return json(route, { checks });
    }

    // --- one-click wirings: each turns its own check green -----------------------
    if (path === "/v1/setup/livetv-connect" && method === "POST") {
      state.checks.livetv = true;
      return json(route, { ok: true });
    }
    if (path === "/v1/setup/tunarr-connect" && method === "POST") {
      state.checks.tunarr_library = true;
      return json(route, { ok: true });
    }

    // --- operator-facing generated credentials (§4) ------------------------------
    if (path.startsWith("/v1/settings/secrets/")) {
      return json(route, { value: "s3cr3t" });
    }

    // --- users --------------------------------------------------------------------
    if (path === "/v1/users/candidates") {
      const rows = CANDIDATES.map((c) => ({ ...c, imported: state.imported.includes(c.id) }));
      return json(route, { candidates: rows });
    }
    if (path === "/v1/users/import" && method === "POST") {
      const ids = (body().ids as string[]) ?? [];
      state.imported.push(...ids);
      return json(route, { imported: ids.length });
    }

    // --- the §7 approval gate ------------------------------------------------------
    // A proposal is created by anyone; only an ADMIN turns it into acquisitions. This
    // mock enforces the same rule the server does, so the smoke proves the UI honors a
    // real 403 rather than a hand-waved one.
    if (path === "/v1/proposals" && method === "POST") {
      if (opts.failedProposalJourney || opts.proposalJourney) {
        const intent = body();
        const prefix = opts.failedProposalJourney ? "failed-job" : "proposal-job";
        const jobId = `${prefix}-${state.proposalJobRequests.length + 1}`;
        state.proposalJobRequests.push(intent);
        return json(route, { jobId });
      }
      const id = `prop-${state.proposals.length + 1}`;
      state.proposals.push({ id, status: "submitted" });
      return json(route, { jobId: `job-${id}` });
    }
    if (path.startsWith("/v1/proposal-jobs/") && path.endsWith("/revise") && method === "POST") {
      const jobId = path.split("/").at(-2) ?? "";
      const intent = body();
      state.proposalRevisionRequests.push({ jobId, intent });
      // The first authoritative read after the accepted mutation is still
      // generating and retains the submitted fallback. A later poll swaps in
      // the replacement, just as the durable Job does in production.
      state.proposalRevisions[jobId] = { intent, pendingJourneyReads: 1 };
      return json(route, { jobId });
    }
    if (path.startsWith("/v1/proposal-jobs/") && method === "GET") {
      const jobId = path.split("/").at(-1) ?? "";
      const submission = state.proposalJobRequests[Number(jobId.split("-").at(-1)) - 1];
      if (opts.failedProposalJourney && submission) {
        return json(route, {
          version: 1,
          jobId,
          milestone: "failed",
          intent: submission,
          attempts: [
            {
              version: 1,
              number: 1,
              status: "failed",
              startedAt: "2026-09-07T12:00:00Z",
              completedAt: "2026-09-07T12:00:01Z",
            },
          ],
          failure: {
            code: "no_grounded_titles",
            reason: "no_catalog_match",
            recoveryAction: "broaden_request",
            message: "No grounded titles matched this request.",
            guidance: "Broaden the request or add examples from your library.",
          },
          actions: ["edit", "retry"],
          createdAt: "2026-09-07T12:00:00Z",
          updatedAt: "2026-09-07T12:00:01Z",
        });
      }
      if (opts.proposalJourney && submission) {
        const revisionState = state.proposalRevisions[jobId];
        const revisionRunning = (revisionState?.pendingJourneyReads ?? 0) > 0;
        if (revisionRunning && revisionState) revisionState.pendingJourneyReads -= 1;
        const replacementReady = revisionState !== undefined && !revisionRunning;
        const currentIntent = revisionState?.intent ?? submission;
        return json(route, {
          version: 1,
          jobId,
          milestone: revisionRunning ? "generating" : "awaiting_approval",
          intent: currentIntent,
          attempts: [
            {
              version: 1,
              number: 1,
              status: "succeeded",
              startedAt: "2026-09-07T12:00:00Z",
              completedAt: "2026-09-07T12:00:01Z",
            },
            ...(revisionState
              ? [
                  {
                    version: 1,
                    number: 2,
                    status: revisionRunning ? "running" : "succeeded",
                    startedAt: "2026-09-07T12:01:00Z",
                    ...(!revisionRunning ? { completedAt: "2026-09-07T12:01:01Z" } : {}),
                  },
                ]
              : []),
          ],
          proposal: {
            id: replacementReady ? `proposal-${jobId}-revision` : `proposal-${jobId}`,
            status: "submitted",
            proposal: {
              intent: replacementReady ? currentIntent : submission,
              channelName: replacementReady ? "Sci-Fi Action Mix" : "Friday Night Action",
              rationale: "A focused night of high-energy 90s action movies.",
              lineup: [
                { name: "Heat", year: 1995, mediaType: "movie", tmdbId: 949, inLibrary: true },
                { name: "Point Break", year: 1991, mediaType: "movie", tmdbId: 1089, inLibrary: true },
              ],
              acquisitions: [
                ...(replacementReady
                  ? [
                      {
                        name: "The Matrix",
                        year: 1999,
                        mediaType: "movie",
                        tmdbId: 603,
                        inLibrary: false,
                      },
                    ]
                  : []),
                { name: "Con Air", year: 1997, mediaType: "movie", tmdbId: 1701, inLibrary: false },
              ],
              alternates: [
                { name: "Face/Off", year: 1997, mediaType: "movie", tmdbId: 754, inLibrary: false },
              ],
              scores: {
                version: 1,
                themeFit: 1,
                availabilityRatio: 1,
                eraBalance: null,
                theme: {
                  status: "supported",
                  basis: "qualifiers",
                  assessedItems: 2,
                  unknownItems: 0,
                  qualifiers: [{ term: "action", supportedItems: 2 }],
                },
                era: { status: "not_requested", assessedItems: 0, matchingItems: 0, unknownItems: 0 },
              },
              trace: { version: 1, surfacedTotal: 1, recordedTotal: 1, truncated: false, candidates: [] },
            },
          },
          actions: revisionRunning ? ["wait"] : ["review", "edit"],
          createdAt: "2026-09-07T12:00:00Z",
          updatedAt: replacementReady ? "2026-09-07T12:01:01Z" : "2026-09-07T12:00:01Z",
        });
      }
    }
    if (/^\/v1\/proposals\/[^/]+\/outlook$/.test(path) && method === "POST") {
      // This mock owns no media inventory. Return an explicit unknown assessment,
      // using the typed shared fixture, rather than the generic empty success body.
      return json(
        route,
        outlook({
          state: "uncertain",
          titles: 1,
          scheduledTitles: 0,
          unknownTitles: 1,
          programs: 0,
          seasons: 0,
          uniqueRuntimeMs: 0,
          firstRepeatMs: null,
          mix: { core: 0, adjacent: 0, discovery: 0, unknown: 1 },
        }),
      );
    }
    if (path === "/v1/search" && method === "GET") {
      return json(route, {
        candidates: [{ name: "The Matrix", year: 1999, mediaType: "movie", tmdbId: 603, inLibrary: false }],
      });
    }
    if (path === "/v1/proposals" && method === "GET") {
      // Shaped as the real ProposalDTO (`proposal.intent.description`, `.rationale`,
      // `.acquisitions`) — the queue reads those exact fields, and a hand-guessed shape
      // would make this smoke pass against a proposal the app can't actually render.
      const wanted = url.searchParams.get("status");
      const rows = state.proposals
        .filter((p) => !wanted || p.status === wanted)
        .map((p) => ({
          id: p.id,
          jobId: `job-${p.id}`,
          status: p.status,
          createdBy: "grace",
          proposal: {
            intent: { description: "90s saturday morning cartoons" },
            rationale: "Kid-friendly 90s animation, all ages.",
            lineup: [{ name: "Animaniacs", year: 1993, mediaType: "series" }],
            // One acquisition: the in-library pick needs nothing, so only the missing
            // title spends anything (§8).
            acquisitions: [{ name: "Gargoyles", year: 1994, mediaType: "series", tmdbId: 12345 }],
            scores: {
              version: 1,
              themeFit: 1,
              availabilityRatio: 0.5,
              eraBalance: null,
              theme: {
                status: "supported",
                basis: "qualifiers",
                assessedItems: 2,
                unknownItems: 0,
                qualifiers: [{ term: "action", supportedItems: 2 }],
              },
              era: { status: "not_requested", assessedItems: 0, matchingItems: 0, unknownItems: 0 },
            },
          },
        }));
      return json(route, { proposals: rows });
    }
    if (path === "/v1/channels" && method === "POST") {
      state.channelCreationRequests.push(body());
      if (state.role !== "admin") {
        return json(route, { title: "Forbidden", detail: "Creating channels is an admin action." }, 403);
      }
      return json(route, { id: `ch-${state.channelCreationRequests.length}` }, 201);
    }
    if (path === "/v1/discovery/feedback" && method === "GET") {
      return json(route, []);
    }
    if (path.endsWith("/approve") && method === "POST") {
      const id = path.split("/").at(-2) ?? "";
      state.approvalRequests.push(id);
      state.approvalEdits.push(body());
      if (state.role !== "admin") {
        return json(route, { title: "Forbidden", detail: "Approving is an admin action." }, 403);
      }
      const found = state.proposals.find((p) => p.id === id);
      if (found) found.status = "approved";
      // Only the not-in-library item becomes an acquisition — the in-library one is
      // already playable and never enters the provisioning loop (§8).
      state.enqueued.push("series:tmdb:gargoyles");
      return json(route, { channelId: "ch-new", enqueued: 1 });
    }
    if (path === "/v1/titles") {
      return json(route, {
        titles: state.enqueued.map((key) => ({ key, mediaType: "series", state: "wanted" })),
      });
    }

    // --- configured Filler shell -------------------------------------------------
    // Page-level browser contracts need complete empty DTOs, not a loose `{}`: the real
    // client deliberately trusts generated response shapes once the capability is enabled.
    if (opts.fillerEnabled) {
      if (path === "/v1/filler/watch") {
        return json(route, {
          health: "attention",
          sourcesOn: 2,
          sourcesReady: 2,
          sourcesTotal: 2,
          clips: 0,
          held: 0,
        });
      }
      if (path === "/v1/filler/attention") return json(route, { rows: [], total: 0 });
      if (path === "/v1/filler/decisions/activity") return json(route, { rows: [], total: 0 });
      if (path === "/v1/filler/decisions/diagnostics") return json(route, { rows: [], total: 0 });
      if (path === "/v1/filler/providers/archive/suggestions" && method === "GET") {
        return json(route, {
          suggestions: [
            {
              provider: "archive",
              targetType: "collection",
              canonicalId: "classic_tv_commercials",
              canonicalUrl: "https://archive.org/details/classic_tv_commercials",
              title: "Classic TV Commercials",
              description: "Public commercials and station breaks",
              itemCount: 8457,
              alreadyAdded: false,
            },
          ],
        });
      }
      if (path === "/v1/filler/providers/archive/resolve" && method === "POST") {
        return json(route, {
          provider: "archive",
          targetType: "collection",
          canonicalId: "classic_tv_commercials",
          canonicalUrl: "https://archive.org/details/classic_tv_commercials",
          title: "Classic TV Commercials",
          itemCount: 8457,
          alreadyAdded: false,
          previewItems: [
            {
              title: "1970s station break",
              url: "https://archive.org/details/station_break_1978",
              durationMs: 31500,
            },
            {
              title: "Local weather bumper",
              url: "https://archive.org/details/weather_bumper",
            },
            {
              title: "Saturday morning promo",
              url: "https://archive.org/details/saturday_promo",
            },
          ],
        });
      }
      if (path === "/v1/filler/providers/youtube/suggestions" && method === "GET") {
        return json(route, {
          suggestions: [
            {
              provider: "youtube",
              targetType: "channel",
              canonicalId: "UC-retro-reels",
              canonicalUrl: "https://www.youtube.com/channel/UC-retro-reels/videos",
              title: "Retro Reels",
              description: "Found from “An hour of vintage station breaks”",
              alreadyAdded: false,
            },
            {
              provider: "youtube",
              targetType: "channel",
              canonicalId: "UC-broadcast-vault",
              canonicalUrl: "https://www.youtube.com/channel/UC-broadcast-vault/videos",
              title: "Broadcast Vault",
              description: "Found from “Classic local commercials”",
              alreadyAdded: false,
            },
          ],
        });
      }
      if (path === "/v1/filler/providers/youtube/resolve" && method === "POST") {
        return json(route, {
          provider: "youtube",
          targetType: "channel",
          canonicalId: "UC-retro-reels",
          canonicalUrl: "https://www.youtube.com/channel/UC-retro-reels/videos",
          title: "Retro Reels",
          itemCount: 24,
          alreadyAdded: false,
          previewItems: [
            {
              title: "An hour of vintage station breaks",
              url: "https://www.youtube.com/watch?v=retro-breaks",
              durationMs: 3600000,
            },
            {
              title: "Classic local commercials",
              url: "https://www.youtube.com/watch?v=local-commercials",
            },
            {
              title: "Network IDs from 1982",
              url: "https://www.youtube.com/watch?v=network-ids",
            },
          ],
        });
      }
      if (path === "/v1/filler/sources/fetch" && method === "POST") {
        const sourceId = url.searchParams.get("id") ?? "";
        state.fillerFetches.push(sourceId);
        return json(route, {
          sourceId,
          sourcesPolled: 1,
          queued: 2,
          skipped: 0,
          maxPerCheck: 10,
          total: 0,
          added: 0,
          updated: 0,
          pruned: 0,
        });
      }
      if (path === "/v1/filler/discover" && method === "GET") {
        const items = Array.from({ length: 20 }, (_, index) => ({
          id: `clip-${index + 1}`,
          title: `Found clip ${index + 1}`,
          url: `https://archive.org/details/clip-${index + 1}`,
          date: "1978-01-01",
        }));
        return json(route, { items, total: items.length, licenceNote: "Check each item's licence." });
      }
      if (path === "/v1/filler/discover/stats" && method === "GET") {
        return json(route, { stats: {} });
      }
      const sourceItemMatch = path.match(/^\/v1\/filler\/sources\/([^/]+)\/items$/);
      if (sourceItemMatch && method === "POST") {
        const request = body() as { remoteId?: unknown; url?: unknown };
        state.fillerSourceItems.push({
          sourceId: decodeURIComponent(sourceItemMatch[1] ?? ""),
          remoteId: String(request.remoteId ?? ""),
          url: String(request.url ?? ""),
        });
        return json(route, { jobId: "e2e-source-item-acquisition" });
      }
      if (path === "/v1/filler/acquisitions/e2e-source-item-acquisition" && method === "GET") {
        const terminal =
          state.fillerAcquisitionStatus === "success" || state.fillerAcquisitionStatus === "error";
        return json(route, {
          id: "e2e-source-item-acquisition",
          trigger: "source",
          sourceId: "archive:long",
          status: state.fillerAcquisitionStatus,
          requested: 1,
          fetched: state.fillerAcquisitionStatus === "success" ? 1 : 0,
          skipped: 0,
          failed: state.fillerAcquisitionStatus === "error" ? 1 : 0,
          empty: 0,
          error: state.fillerAcquisitionStatus === "error" ? "Archive.org timed out" : undefined,
          startedAt: "2026-09-13T14:00:00Z",
          completedAt: terminal ? "2026-09-13T14:01:00Z" : undefined,
          updatedAt: terminal ? "2026-09-13T14:01:00Z" : "2026-09-13T14:00:00Z",
          outcome: {
            enrolled: state.fillerAcquisitionStatus === "success" ? 1 : 0,
            preparing: state.fillerAcquisitionStatus === "success" ? 1 : 0,
            needsDecision: 0,
            admitted: 0,
            rejected: 0,
            dismissed: 0,
          },
          artifacts: {
            staged: 0,
            published: 0,
            consumed: state.fillerAcquisitionStatus === "success" ? 1 : 0,
            repair: 0,
          },
        });
      }
      if (path === "/v1/filler/ingest" && method === "POST") {
        return json(route, { jobId: "e2e-filler-ingest" });
      }
      if (path === "/v1/filler/sources" && method === "POST") {
        return json(route, {
          id: "archive:classic_tv_commercials",
          uri: "classic_tv_commercials",
          label: "Classic TV Commercials",
          enabled: true,
        });
      }
      const sourcePatchMatch = path.match(/^\/v1\/filler\/sources\/(.+)$/);
      if (sourcePatchMatch && method === "PATCH") {
        const patch = body();
        state.fillerSourcePatches.push({
          id: decodeURIComponent(sourcePatchMatch[1] ?? ""),
          body: patch,
        });
        if (patch.automaticDownloads) {
          state.fillerSourcePolicy = patch.automaticDownloads as typeof state.fillerSourcePolicy;
        }
        return json(route, {});
      }
      if (path === "/v1/filler/sources") {
        const inheritedEvery = state.edits["filler.fetch.every"] ?? "6h";
        const inheritedEverySeconds = inheritedEvery === "0" ? 0 : Number.parseInt(inheritedEvery, 10) * 3600;
        const inheritedMax = Number(state.edits["filler.fetch.max_per_run"] ?? "10");
        const policy =
          state.fillerSourcePolicy.mode === "defaults"
            ? {
                mode: "defaults" as const,
                everySeconds: inheritedEverySeconds,
                maxPerCheck: inheritedMax,
              }
            : state.fillerSourcePolicy;
        const policySummary =
          policy.mode === "never"
            ? "Doesn’t download automatically. You can still look for clips yourself."
            : `${policy.mode === "defaults" ? "Uses your defaults: e" : "E"}very ${policy.everySeconds / 3600} hours, up to ${policy.maxPerCheck} clips each check.`;
        return json(route, {
          sources: [
            {
              id: "folder",
              uri: "/data/filler/a-deliberately-long-folder-name",
              kind: "folder",
              target: "/data/filler/a-deliberately-long-folder-name",
              detail: "watched directly — new files appear on the next pass",
              count: 0,
              configured: true,
              fetchable: true,
              enabled: true,
              effectiveEnabled: true,
              providerEnabled: true,
              switchable: true,
              removable: false,
              searchable: false,
              readiness: "ready",
              ready: true,
              locationSource: "installation",
              actions: ["fetch", "disable"],
            },
            {
              id: "provider:archive",
              kind: "archive",
              target: "Archive.org",
              detail: "collections you added",
              count: 0,
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
              id: "archive:long",
              uri: "classic_tv_commercials",
              kind: "archive",
              target: "Classic television commercials from a deliberately long collection name",
              detail: "an archive.org collection",
              count: 0,
              configured: true,
              fetchable: true,
              enabled: true,
              effectiveEnabled: true,
              providerEnabled: true,
              switchable: true,
              removable: true,
              searchable: true,
              parentId: "provider:archive",
              readiness: "ready",
              ready: true,
              locationSource: "installation",
              automaticDownloads: {
                ...policy,
                summary: policySummary,
                ...(policy.mode === "never" ? {} : { nextCheckAt: "2026-09-13T18:00:00Z" }),
              },
              actions: ["fetch", "search", "disable", "remove", "edit_location"],
            },
          ],
          total: 0,
        });
      }
      if (path === "/v1/filler/pool") {
        return json(route, { clips: 0, commercials: 0, eligible: 0, untagged: 0, channels: [] });
      }
      if (path === "/v1/filler/incoming") {
        return json(route, {
          clips: [],
          reels: [],
          rejected: [],
          stageOrder: [],
          clipsTotal: 0,
          decisionsTotal: 0,
          reelsTotal: 0,
          rejectedTotal: 0,
        });
      }
      if (path === "/v1/filler/decisions/overview") {
        return json(route, {
          healthy: true,
          nextAction: "none",
          actionCount: 0,
          counts: {
            admitted: 0,
            rejected: 0,
            reviews: 0,
            unresolvedReviews: 0,
            operational: 0,
            retryable: 0,
          },
        });
      }
      if (path === "/v1/filler/readiness") {
        return json(route, {
          ready: false,
          nextAction: "add_filler",
          fetch: { enabled: false, catalogClips: 0 },
          pipeline: {
            runnable: 0,
            scheduled: 0,
            inProgress: 0,
            needsDecision: 0,
            recoverable: 0,
            admitted: 0,
            rejected: 0,
            dismissed: 0,
          },
          pool: { clips: 0, commercials: 0, eligible: 0, untagged: 0, channels: [] },
          acquisitions: [],
        });
      }
      if (path === "/v1/filler") return json(route, { clips: [], total: 0 });
    }

    // --- settings (the wizard's terminal act writes setup.completed here) ---------
    if (path === "/v1/settings" && method === "PATCH") {
      Object.assign(state.edits, (body().edits as Record<string, string>) ?? {});
      const results = Object.keys(state.edits).map((key) => ({ key, status: "saved" }));
      return json(route, { results });
    }
    if (path === "/v1/locations" && method === "GET") {
      const query = (url.searchParams.get("q") ?? "").toLowerCase();
      const locations = [
        { id: "5128581", label: "New York City, United States", country: "US", market: "New York City" },
        {
          id: "5129061",
          label: "North Greenbush, United States",
          country: "US",
          market: "North Greenbush",
          region: "New York",
        },
        { id: "country-US", label: "United States", country: "US" },
        { id: "2643743", label: "London, United Kingdom", country: "GB", market: "London" },
      ].filter((location) => location.label.toLowerCase().includes(query));
      return json(route, { locations });
    }
    if (path === "/v1/locations/resolve" && method === "POST") {
      return json(route, {
        location: {
          id: "5128581",
          label: "New York City, United States",
          country: "US",
          market: "New York City",
          source: "device",
        },
      });
    }
    if (path === "/v1/locations/suggestion" && method === "GET") {
      return json(route, {
        suggestion: {
          id: "country-US",
          label: "United States",
          country: "US",
          source: "proxy",
          approximate: true,
        },
      });
    }
    if (path === "/v1/settings") {
      // A field per connection group so the Connections step (config-design §6) renders its
      // inline forms — otherwise the blocks are empty and the flow snapshot lies about the
      // real UI. One essential key each is enough; the reveal/sub-nav needs the groups.
      const connEntry = (key: string, group: string, doc: string) => ({
        key,
        group,
        kind: "string",
        doc,
        advanced: false,
        secret: false,
        set: false,
        provenance: "db" as const,
        value: state.edits[key] ?? "",
      });
      return json(route, {
        features: opts.fillerEnabled ? { filler: true } : {},
        settings: [
          connEntry("media_server.url", "connections.media_server", "Media server base URL."),
          connEntry("media_server.token", "connections.media_server", "Media server API token."),
          connEntry("tunarr.url", "connections.tunarr", "Tunarr base URL."),
          connEntry("seerr.url", "connections.requester", "Seerr base URL."),
          connEntry("tmdb.api_key", "connections.tmdb", "TMDB API key."),
          {
            // Internal playout cannot publish a tuner until the media server has a
            // machine-reachable Loomarr address. Keep this empty on the first visit so
            // the e2e snapshot proves the default path shows the required field and does
            // not silently treat the backend default as a complete answer.
            key: "server.public_url",
            label: "Loomarr address",
            group: "playout",
            kind: "url",
            doc: "Loomarr's address as the media server can reach it.",
            advanced: false,
            secret: false,
            set: Boolean(state.edits["server.public_url"]),
            provenance: "db",
            value: state.edits["server.public_url"] ?? "",
          },
          {
            // Who plays the channels (§9.1). Reads back whatever the wizard last PATCHed, so
            // picking Tunarr in the walk genuinely reshapes the remaining steps rather than
            // being a click the mock ignores.
            key: "playout.backend",
            group: "playout",
            kind: "enum",
            enum: ["internal", "tunarr"],
            doc: "Who streams a channel.",
            advanced: false,
            secret: false,
            set: true,
            provenance: "db",
            value: state.edits["playout.backend"] ?? "internal",
          },
          {
            key: "setup.completed",
            group: "advanced",
            kind: "bool",
            doc: "First-run wizard completed.",
            advanced: true,
            secret: false,
            set: true,
            provenance: "db",
            value: state.edits["setup.completed"] ?? "false",
          },
          {
            key: "filler.home_country",
            label: "Country",
            group: "filler",
            kind: "string",
            doc: "Country where the channels are watched.",
            advanced: false,
            secret: false,
            set: Boolean(state.edits["filler.home_country"]),
            provenance: "db",
            value: state.edits["filler.home_country"] ?? "",
          },
          {
            key: "filler.home_market",
            label: "Local area",
            group: "filler",
            kind: "string",
            doc: "Local area where the channels are watched.",
            advanced: false,
            secret: false,
            set: Boolean(state.edits["filler.home_market"]),
            provenance: "db",
            value: state.edits["filler.home_market"] ?? "",
          },
          ...(opts.fillerEnabled
            ? [
                {
                  key: "filler.dir",
                  group: "filler",
                  kind: "path",
                  doc: "Filler library directory.",
                  advanced: false,
                  secret: false,
                  set: true,
                  provenance: "db" as const,
                  value: "/data/filler",
                },
                {
                  key: "filler.fetch.every",
                  label: "Look for new clips",
                  group: "filler",
                  kind: "duration",
                  presentation: "filler_download_schedule",
                  doc: "How often Loomarr checks enabled sources for new clips.",
                  advanced: false,
                  secret: false,
                  set: true,
                  provenance: "db" as const,
                  value: state.edits["filler.fetch.every"] ?? "6h0m0s",
                },
                {
                  key: "filler.fetch.max_per_run",
                  label: "Add up to",
                  group: "filler",
                  kind: "int",
                  doc: "The most clips each enabled source may add in one automatic check.",
                  advanced: false,
                  secret: false,
                  set: true,
                  provenance: "db" as const,
                  value: state.edits["filler.fetch.max_per_run"] ?? "10",
                },
                {
                  key: "filler.fetch.max_catalog_clips",
                  label: "Automatic-download catalog limit",
                  group: "filler",
                  kind: "int",
                  doc: "Stop downloading automatically once the catalog reaches this size.",
                  advanced: true,
                  secret: false,
                  set: true,
                  provenance: "db" as const,
                  value: "2000",
                },
                {
                  key: "filler.fetch.max_disk_gb",
                  label: "Automatic-download storage limit (GB)",
                  group: "filler",
                  kind: "int",
                  doc: "Stop downloading automatically once filler storage reaches this size.",
                  advanced: true,
                  secret: false,
                  set: true,
                  provenance: "db" as const,
                  value: "20",
                },
              ]
            : []),
        ],
      });
    }

    // Anything the wizard doesn't need answers empty rather than 404-ing, so an
    // unrelated background query can't fail a flow assertion.
    return json(route, {});
  });

  return { state };
};

export type { MockBackend, MockOptions };
export { installMockBackend };
