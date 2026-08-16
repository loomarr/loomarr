import type { Page, Route } from "@playwright/test";

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
  // Which setup/status checks are green before the operator does anything. The two
  // REQUIRED ones default green so the flow can reach the wiring steps.
  checks?: Record<string, boolean>;
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
      const id = `prop-${state.proposals.length + 1}`;
      state.proposals.push({ id, status: "submitted" });
      return json(route, { jobId: `job-${id}` });
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
            scores: { themeFit: 0.9, availabilityRatio: 0.5, coherence: 0.8 },
          },
        }));
      return json(route, { proposals: rows });
    }
    if (path.endsWith("/approve") && method === "POST") {
      if (state.role !== "admin") {
        return json(route, { title: "Forbidden", detail: "Approving is an admin action." }, 403);
      }
      const id = path.split("/").at(-2) ?? "";
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

    // --- settings (the wizard's terminal act writes setup.completed here) ---------
    if (path === "/v1/settings" && method === "PATCH") {
      Object.assign(state.edits, (body().edits as Record<string, string>) ?? {});
      const results = Object.keys(state.edits).map((key) => ({ key, status: "saved" }));
      return json(route, { results });
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
        features: {},
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
