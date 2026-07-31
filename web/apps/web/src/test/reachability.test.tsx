import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

// REACHABILITY — the gate this phase earned.
//
// Seven times in 13.4 something was built, unit-tested, and unreachable: two settings
// panels never mounted; a formatter never called, so "·til 8:00 PM" was dead UI on every
// channel card; a 323-line settings form rendered by nothing; a clip's tag action gated
// so the one clip that needed correcting couldn't be; a search scope that always returned
// empty; a Search button wired to a discarded setState. Every component test passed in
// every case, because a component test cannot see whether anything mounts it.
//
// So this asserts REACHABILITY rather than behavior: every route in the generated tree
// renders real content, and every feature-gated panel appears when its flag is on. The
// route list is derived from the router itself — a hand-maintained list is the same
// mistake one level up (see structure.test.ts, which learned this the hard way).

// `local: true` mirrors the meBody field (§11 credential path) — the Account screen
// offers a password form only for a Loomarr-stored credential, so a fixture without it
// would silently exercise the media-server branch instead.
const ADMIN = {
  id: "u1",
  name: "Ada",
  role: "admin",
  autoApprove: true,
  disabled: false,
  quota: 0,
  local: true,
};

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

// Every feature on, so gated panels are expected to appear rather than explain themselves.
const FEATURES = {
  filler: true,
  suggestions: true,
  acquisition: true,
  user_sync: true,
  ingest: true,
};

const stubFetch = () => {
  const mock = vi.fn((url: string) => {
    const u = String(url);
    if (u.includes("/v1/auth/me")) return Promise.resolve(json(ADMIN));
    if (u.includes("/v1/setup/status")) return Promise.resolve(json({ checks: [] }));
    if (u.includes("/v1/settings/secrets")) return Promise.resolve(json({ value: "" }));
    if (u.includes("/v1/settings")) {
      return Promise.resolve(
        json({
          features: FEATURES,
          settings: [
            {
              key: "library.url",
              group: "connections.media_server",
              kind: "url",
              doc: "help",
              advanced: false,
              secret: false,
              set: true,
              provenance: "db",
              value: "http://emby:8096",
            },
            {
              key: "filler.dir",
              group: "filler",
              kind: "string",
              doc: "help",
              advanced: false,
              secret: false,
              set: true,
              provenance: "db",
              value: "/filler",
            },
            {
              key: "job.workers",
              group: "advanced",
              kind: "int",
              doc: "help",
              advanced: false,
              secret: false,
              set: true,
              provenance: "db",
              value: "2",
            },
            {
              key: "channel.reconcile_every",
              group: "channels",
              kind: "duration",
              doc: "help",
              advanced: false,
              secret: false,
              set: true,
              provenance: "db",
              value: "5m",
            },
            {
              key: "session.ttl",
              group: "users_security",
              kind: "duration",
              doc: "help",
              advanced: false,
              secret: false,
              set: true,
              provenance: "db",
              value: "720h",
            },
            {
              key: "llm.url",
              group: "ai",
              kind: "url",
              doc: "help",
              advanced: false,
              secret: false,
              set: true,
              provenance: "db",
              value: "http://ollama:11434",
            },
          ],
        }),
      );
    }
    if (u.includes("/v1/system/llm")) {
      return Promise.resolve(
        json({
          local: true,
          reachable: true,
          provider: "ollama",
          model: "qwen3:8b",
          catalog: [],
          hosted: [],
        }),
      );
    }
    if (u.includes("/v1/system/version")) {
      return Promise.resolve(json({ version: "dev", ready: true }));
    }
    if (u.includes("/v1/users/candidates")) return Promise.resolve(json({ candidates: [] }));
    if (u.includes("/sessions")) return Promise.resolve(json({ sessions: [] }));
    if (u.includes("/v1/users")) return Promise.resolve(json({ users: [{ ...ADMIN, local: true }] }));
    if (u.includes("/v1/docs/")) {
      return Promise.resolve(
        json({ slug: "troubleshooting", title: "Troubleshooting", markdown: "# Troubleshooting\n\nHere." }),
      );
    }
    if (u.includes("/v1/docs"))
      return Promise.resolve(json({ docs: [{ slug: "troubleshooting", title: "Troubleshooting" }] }));
    // Before the /v1/filler catalog match below: this path contains "/filler" but is the
    // per-CHANNEL coverage route, and a clips payload would not satisfy the meter.
    if (u.includes("/filler/coverage")) {
      return Promise.resolve(json({ level: "exact", total: 4, rungs: [{ level: "exact", clips: 4 }] }));
    }
    if (u.includes("/v1/filler")) return Promise.resolve(json({ clips: [] }));
    if (u.includes("/v1/channels/now-next")) return Promise.resolve(json({ channels: [] }));
    if (u.includes("/pods")) return Promise.resolve(json({ entries: [], totalMs: 0, matchLevel: "exact" }));
    if (u.includes("/v1/channels/")) {
      return Promise.resolve(
        json({
          id: "ch-1",
          name: "Cartoons",
          number: 42,
          status: "live",
          strategy: "shuffle",
          programCount: 3,
          pendingCount: 1,
          breakCount: 0,
          slotCount: 4,
          policy: {},
        }),
      );
    }
    if (u.includes("/v1/channels")) {
      return Promise.resolve(
        json({
          channels: [
            {
              id: "ch-1",
              name: "Cartoons",
              number: 42,
              status: "live",
              strategy: "shuffle",
              programCount: 3,
              pendingCount: 1,
              breakCount: 0,
              slotCount: 4,
              policy: {},
            },
          ],
        }),
      );
    }
    if (u.includes("/v1/titles")) return Promise.resolve(json({ titles: [] }));
    if (u.includes("/v1/proposals")) return Promise.resolve(json({ proposals: [] }));
    if (u.includes("/v1/suggestions")) return Promise.resolve(json({ proposals: [] }));
    return Promise.resolve(json({}));
  });
  vi.stubGlobal("fetch", mock);
  vi.stubGlobal(
    "EventSource",
    class {
      addEventListener() {}
      close() {}
    },
  );
};

const renderAt = (path: string) => {
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
};

// Turn a generated route id into a navigable path: strip pathless layout segments
// (`_authed`), drop the trailing index marker, and fill params with a value the stub
// serves. Deriving these from the router means a NEW route is covered the day it lands.
const pathOf = (id: string): string => {
  const path = id
    .split("/")
    .filter((seg) => seg !== "" && !seg.startsWith("_"))
    .map((seg) => (seg.startsWith("$") ? "ch-1" : seg))
    .join("/");
  return `/${path}`;
};

const routeIds = (): string[] => {
  const queryClient = new QueryClient();
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  return Object.keys(router.routesById).filter(
    (id) =>
      id !== "__root__" &&
      // Layout routes render only an <Outlet>; their children are covered individually.
      id !== "/_authed" &&
      id !== "/_authed/settings" &&
      // The catch-all is SUPPOSED to be a placeholder — it is the 404 page.
      id !== "/_authed/$" &&
      // Login and the wizard redirect an authenticated admin away; they have their own
      // suites (auth + wizard) that drive them unauthenticated.
      id !== "/login" &&
      id !== "/wizard",
  );
};

afterEach(() => vi.restoreAllMocks());

describe("every route is reachable", () => {
  it.each(routeIds().map((id) => [id, pathOf(id)] as const))("%s renders real content", async (_id, path) => {
    stubFetch();
    renderAt(path);

    // A heading proves the screen composed, not just that the shell painted around an
    // empty pane. The shell's own nav has no headings, so this cannot pass vacuously.
    await waitFor(
      () => {
        const headings = screen.getAllByRole("heading");
        expect(headings.length).toBeGreaterThan(0);
      },
      { timeout: 3000 },
    );

    // The catch-all's copy appearing anywhere else means the path did not match the
    // route we think it did.
    expect(screen.queryByText("Off the air")).not.toBeInTheDocument();
  });
});

describe("feature-gated panels mount when their flag is on", () => {
  // Each entry names a panel that EXISTS as a component but has, at least once in this
  // phase, not been rendered by the page that owns it.
  it.each([
    ["/settings/ai", /probing your llm host|model|provider/i, "the §8.1 model picker"],
    ["/settings/security", /api token|session secret/i, "the generated-secrets panel"],
    // ⚠ The SSO block AND the note stating what SSO does not do (§11, V8). The note is the
    // part most likely to be lost in a tidy-up — it looks like prose rather than a control —
    // and losing it leaves §11's unusual model (most apps DO auto-create) reading as an
    // oversight.
    ["/settings/security", /does not create an account here/i, "the SSO scope note"],
    ["/people", /import from your media server/i, "the §11 import panel"],
    ["/filler", /download clips/i, "the ingest panel"],
  ])("%s mounts %s", async (path, pattern) => {
    stubFetch();
    renderAt(path);
    // findAllBy, not findBy: a panel legitimately renders its name more than once (a
    // heading plus a row label). Presence is the assertion here, not uniqueness.
    const found = await screen.findAllByText(pattern, undefined, { timeout: 3000 });
    expect(found.length).toBeGreaterThan(0);
  });

  // The §12 pod preview lives in the channel-detail "Filler" tab (admin-only) — the detail page
  // is now a tabbed layout, one section shown at a time. This guards the tab is WIRED +
  // reachable: the Filler tab is present, and selecting it reveals the live draft-sandbox break.
  it("/channels/ch-1 reaches the §12 filler section (its tab) with the break preview", async () => {
    stubFetch();
    renderAt("/channels/ch-1");
    // The Filler tab in the section bar is always present for an admin.
    const fillerTab = await screen.findByRole("button", { name: "Filler" });
    expect(fillerTab).toBeInTheDocument();
    // Selecting it swaps to the Filler panel → the break preview inside is reachable.
    await userEvent.click(fillerTab);
    const found = await screen.findAllByText(/this channel's break/i, undefined, { timeout: 3000 });
    expect(found.length).toBeGreaterThan(0);
  });

  // V29b's meter, in the same tab. Guarded here rather than trusted because this suite exists
  // for exactly this shape of miss: the component has stories, six unit tests and a Go test
  // proving it agrees with pod assembly, and every one of those passes whether or not anything
  // renders it. Only a route test answers "can an operator see it".
  it("/channels/ch-1 reaches the filler coverage meter", async () => {
    stubFetch();
    renderAt("/channels/ch-1");
    await userEvent.click(await screen.findByRole("button", { name: "Filler" }));
    expect(await screen.findByText(/catalog coverage/i, undefined, { timeout: 3000 })).toBeInTheDocument();
    // And the meter itself rendered, not just its heading.
    expect(await screen.findByText("Exact match")).toBeInTheDocument();
  });

  // The eighth instance of this file's founding bug: ChannelIconField shipped complete —
  // stories, five visual baselines, an admin gate — and was imported by nothing, so the
  // channel icon was unreachable in the app. Its component tests all passed, which is
  // exactly the blind spot this suite exists for.
  it("/channels/ch-1 reaches the channel icon field on the info panel", async () => {
    stubFetch();
    renderAt("/channels/ch-1");
    // Info is the default panel (and the viewer's only one), so no tab click is needed.
    expect(await screen.findByText("Channel icon")).toBeInTheDocument();
  });

  // The ninth instance, and the one this suite failed to prevent: V7 shipped
  // POST /v1/auth/password with 19 tests and no screen, so a user still could not
  // change their password by clicking anything. A route test is the gate — the
  // endpoint being correct was never the question.
  it("/account reaches the change-password form for a local user", async () => {
    stubFetch();
    renderAt("/account");
    expect(await screen.findByText("Your account")).toBeInTheDocument();
    expect(await screen.findByLabelText("Current password")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /change password/i })).toBeInTheDocument();
  });
});
