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
  // Which setup/status checks are green before the operator does anything. The two
  // REQUIRED ones default green so the flow can reach the wiring steps.
  checks?: Record<string, boolean>;
  // Per-app webhook receipts; the wizard renders these as "Last received …".
  webhook?: Record<string, string>;
}

interface MockBackend {
  // What the run recorded — the smoke asserts against these rather than re-reading UI.
  readonly state: {
    authed: boolean;
    checks: Record<string, boolean>;
    imported: string[];
    edits: Record<string, string>;
  };
}

const json = (route: Route, body: unknown, status = 200) =>
  route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });

const installMockBackend = async (page: Page, opts: MockOptions = {}): Promise<MockBackend> => {
  const state = {
    authed: opts.authed ?? false,
    checks: { media_server: true, tunarr: true, ...(opts.checks ?? {}) } as Record<string, boolean>,
    webhook: { ...(opts.webhook ?? {}) } as Record<string, string>,
    imported: [] as string[],
    edits: {} as Record<string, string>,
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
      return state.authed ? json(route, ADMIN) : json(route, { title: "Unauthorized" }, 401);
    }
    if (path === "/v1/auth/login" && method === "POST") {
      state.authed = true;
      return json(route, ADMIN);
    }
    if (path === "/v1/setup/bootstrap" && method === "POST") {
      return json(route, { id: ADMIN.id, name: ADMIN.name, role: ADMIN.role });
    }

    // --- the checklist the whole wizard is derived from --------------------------
    if (path === "/v1/setup/status") {
      const checks = Object.entries(state.checks).map(([name, ok]) => ({
        name,
        ok,
        hint: ok ? undefined : `${name} is not configured yet.`,
      }));
      checks.push({ name: "webhook", ok: Object.keys(state.webhook).length > 0, hint: undefined });
      const withReceipts = checks.map((c) =>
        c.name === "webhook" ? { ...c, lastReceived: state.webhook } : c,
      );
      return json(route, { checks: withReceipts });
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

    // --- the webhook panel's secret (revealed, never rotated — §4) ---------------
    if (path.startsWith("/v1/settings/secrets/")) {
      const name = path.split("/").pop() ?? "";
      return json(
        route,
        name === "session_secret" ? { displayable: false } : { value: "s3cr3t", displayable: true },
      );
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

    // --- settings (the wizard's terminal act writes setup.completed here) ---------
    if (path === "/v1/settings" && method === "PATCH") {
      Object.assign(state.edits, (body().edits as Record<string, string>) ?? {});
      const results = Object.keys(state.edits).map((key) => ({ key, status: "saved" }));
      return json(route, { results });
    }
    if (path === "/v1/settings") {
      return json(route, {
        features: {},
        settings: [
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
