import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

// Wizard coverage driven through the REAL generated route tree (§13, config-design §6):
// first-run routing off `setup.completed`, the unauthenticated bootstrap step, and the
// bootstrap → auto-login → checklist advance.
const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };
const GREEN_CHECKS = [
  { name: "media_server", ok: true },
  { name: "tunarr", ok: true },
  { name: "llm", ok: false, hint: "No LLM configured — suggestions stay off until you connect one." },
];

// The connections step (config-design §6) renders the settings GROUP forms, so the
// settings list must carry a field per group for its blocks to appear. One essential key
// each is enough to exercise the block headings + Test/verdict.
const entry = (key: string, group: string) => ({
  key,
  group,
  kind: "string",
  doc: `${key} for tests`,
  advanced: false,
  secret: false,
  set: false,
  provenance: "db",
  value: "",
});
// An enum entry renders as the Radix Select — the control the flavor-save regression
// (below) exercises. `enum` fills its options; the jsdom Select shims live in test/setup.ts.
const enumEntry = (key: string, group: string, options: string[]) => ({
  ...entry(key, group),
  kind: "enum",
  enum: options,
});
const CONNECTION_ENTRIES = [
  enumEntry("library.flavor", "connections.media_server", ["emby", "jellyfin"]),
  entry("media_server.url", "connections.media_server"),
  entry("tunarr.url", "connections.tunarr"),
  entry("seerr.url", "connections.requester"),
  entry("tmdb.api_key", "connections.tmdb"),
];

const json = (body: unknown, status: number) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stubFetch = (opts: { authed: boolean; setupCompleted?: boolean; checks?: unknown[] }) => {
  let authed = opts.authed;
  const mock = vi.fn((url: string, _init?: RequestInit) => {
    const u = String(url);
    if (u.includes("/v1/setup/bootstrap")) return Promise.resolve(json(ADMIN, 200));
    if (u.includes("/v1/auth/login")) {
      authed = true;
      return Promise.resolve(json(ADMIN, 200));
    }
    if (u.includes("/v1/auth/me")) {
      return Promise.resolve(authed ? json(ADMIN, 200) : json({ title: "Unauthorized" }, 401));
    }
    if (u.includes("/v1/setup/status")) {
      return Promise.resolve(json({ checks: opts.checks ?? GREEN_CHECKS }, 200));
    }
    if (u.includes("/v1/setup/test")) {
      // The real endpoint tests PERSISTED settings; the FE must save first, so the mock
      // just acks. The flavor-save regression asserts the PATCH landed BEFORE this call.
      return Promise.resolve(json({ ok: true, hint: "Connection OK" }, 200));
    }
    if (u.includes("/v1/settings")) {
      return Promise.resolve(
        json(
          {
            features: {},
            settings: [
              ...CONNECTION_ENTRIES,
              {
                key: "setup.completed",
                group: "advanced",
                kind: "bool",
                doc: "First-run wizard completed.",
                advanced: true,
                secret: false,
                set: true,
                provenance: "db",
                value: String(opts.setupCompleted ?? true),
              },
            ],
          },
          200,
        ),
      );
    }
    return Promise.resolve(json({}, 200));
  });
  vi.stubGlobal("fetch", mock);
  return mock;
};

const renderAt = (initialPath: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

afterEach(() => vi.restoreAllMocks());

describe("first-run routing", () => {
  it("sends the operator to the wizard while setup.completed is false", async () => {
    stubFetch({ authed: true, setupCompleted: false });
    renderAt("/");
    expect(await screen.findByText(/first-run setup/i)).toBeInTheDocument();
  });

  it("goes straight to Channels once setup is completed", async () => {
    stubFetch({ authed: true, setupCompleted: true });
    renderAt("/");
    expect(await screen.findByRole("heading", { name: "Channels" })).toBeInTheDocument();
  });
});

describe("wizard", () => {
  it("opens on bootstrap when no one is signed in", async () => {
    stubFetch({ authed: false });
    renderAt("/wizard");
    expect(await screen.findByRole("heading", { name: /create your admin account/i })).toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toBeInTheDocument();
    expect(screen.getByLabelText("Confirm password")).toBeInTheDocument();
  });

  it("creates the admin, signs in, and advances to the checklist", async () => {
    const fetchMock = stubFetch({ authed: false });
    renderAt("/wizard");

    await userEvent.type(await screen.findByLabelText("Username"), "ada");
    await userEvent.type(screen.getByLabelText("Password"), "hunter2!");
    await userEvent.type(screen.getByLabelText("Confirm password"), "hunter2!");
    await userEvent.click(screen.getByRole("button", { name: /create admin/i }));

    expect(await screen.findByRole("heading", { name: /connect your services/i })).toBeInTheDocument();
    const called = (path: string) => fetchMock.mock.calls.some(([u]) => String(u).includes(path));
    expect(called("/v1/setup/bootstrap")).toBe(true);
    expect(called("/v1/auth/login")).toBe(true);
  });

  it("rejects a mismatched confirmation before calling the API", async () => {
    const fetchMock = stubFetch({ authed: false });
    renderAt("/wizard");

    await userEvent.type(await screen.findByLabelText("Username"), "ada");
    await userEvent.type(screen.getByLabelText("Password"), "hunter2!");
    await userEvent.type(screen.getByLabelText("Confirm password"), "different!");
    await userEvent.click(screen.getByRole("button", { name: /create admin/i }));

    expect(await screen.findByText(/passwords don't match/i)).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/v1/setup/bootstrap"))).toBe(false);
  });

  it("shows the connections as inline forms and blocks while a required one is red", async () => {
    stubFetch({
      authed: true,
      setupCompleted: false,
      checks: [
        { name: "media_server", ok: true },
        { name: "tunarr", ok: false, hint: "Tunarr didn't answer on that URL." },
      ],
    });
    renderAt("/wizard");

    // Each connection is a collapsible block AND a rail sub-item (§13 sub-nav), so the
    // names appear twice — that is the point, not a bug.
    expect(await screen.findAllByText("Media server")).not.toHaveLength(0);
    const rail = within(screen.getByRole("complementary"));
    const tunarrSubItem = rail.getByRole("button", { name: "Tunarr" });

    // The connections step renders the settings-group FORM, not a read-only checklist —
    // configure in place (§6). A Test-connection button per block is the tell (there is no
    // "Fix ↗ go to Settings" here anymore).
    expect(screen.getAllByRole("button", { name: /test connection/i }).length).toBeGreaterThan(0);

    // Opening Tunarr from the rail reveals its red verdict — the BE's plain-language hint,
    // never a stack trace (§13) — and Continue stays disabled while a required check is red.
    await userEvent.click(tunarrSubItem);
    expect(await screen.findByText("Tunarr didn't answer on that URL.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();
  });

  it("saves the flavor before testing — Test checks what's on screen, not stale settings", async () => {
    // Regression: /v1/setup/test evaluates PERSISTED settings, so testing an unsaved edit
    // ran against the OLD (empty) flavor — picking "emby" then Testing showed "set a media
    // server flavor". Test must PATCH the dirty edits first, THEN test. Asserted by call
    // ORDER: the PATCH (carrying library.flavor) must precede the /v1/setup/test POST.
    const fetchMock = stubFetch({
      authed: true,
      setupCompleted: false,
      // Both required checks red so the wizard STAYS on the connections step (a
      // satisfied checklist auto-advances — see the "resumes past" test below).
      checks: [
        { name: "media_server", ok: false, hint: "Not connected yet" },
        { name: "tunarr", ok: false, hint: "Not connected yet" },
      ],
    });
    renderAt("/wizard");

    // Wait for the connections step to mount (auth + route resolve async), then open
    // the media-server block from the rail and pick a flavor in the Radix Select.
    await screen.findByRole("heading", { name: /connect your services/i });
    const rail = within(screen.getByRole("complementary"));
    await userEvent.click(rail.getByRole("button", { name: "Media server" }));
    await userEvent.click(await screen.findByRole("combobox", { name: /library flavor/i }));
    await userEvent.click(await screen.findByRole("option", { name: "emby" }));

    // The media-server block is the one open, so its Test button is the first.
    const [testButton] = screen.getAllByRole("button", { name: /test connection/i });
    if (!testButton) throw new Error("no Test connection button rendered");
    await userEvent.click(testButton);

    // The save landed before the test — and carried the flavor the operator just picked.
    await vi.waitFor(() => {
      const patch = fetchMock.mock.calls.find(
        ([u, init]) => String(u).includes("/v1/settings") && (init as RequestInit)?.method === "PATCH",
      );
      const testCall = fetchMock.mock.calls.find(([u]) => String(u).includes("/v1/setup/test"));
      expect(patch).toBeDefined();
      expect(testCall).toBeDefined();
      const patchIdx = fetchMock.mock.calls.indexOf(patch as (typeof fetchMock.mock.calls)[number]);
      const testIdx = fetchMock.mock.calls.indexOf(testCall as (typeof fetchMock.mock.calls)[number]);
      expect(patchIdx).toBeLessThan(testIdx);
      expect(String((patch?.[1] as RequestInit)?.body)).toContain("library.flavor");
    });
  });

  it("resumes past the checklist when only optional integrations are red", async () => {
    // media_server + tunarr green, llm red: the checklist is satisfied (config-design §6
    // — Seerr/AI/TMDB are feature-gating, not blocking), so the wizard moves the operator
    // on to the next unfinished step rather than stranding them on a red X.
    stubFetch({ authed: true, setupCompleted: false });
    renderAt("/wizard");

    expect(
      await screen.findByRole("heading", { name: /put your channels in the tv guide/i }),
    ).toBeInTheDocument();
  });
});
