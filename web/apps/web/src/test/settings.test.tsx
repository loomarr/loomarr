import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };

const entry = (over: Record<string, unknown>) => ({
  group: "connections.media_server",
  kind: "string",
  doc: "help",
  advanced: false,
  secret: false,
  set: true,
  provenance: "db",
  ...over,
});

const SETTINGS = [
  entry({ key: "library.url", kind: "url", value: "http://emby:8096" }),
  entry({ key: "library.token", kind: "secret", secret: true, preview: "…a1b2", value: "" }),
  entry({ key: "tunarr.url", group: "connections.tunarr", kind: "url", value: "http://tunarr:8000" }),
  entry({ key: "job.workers", group: "advanced", kind: "int", value: "2", provenance: "env" }),
];

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stubFetch = () => {
  const mock = vi.fn((url: string, init?: RequestInit) => {
    const u = String(url);
    if (u.includes("/v1/auth/me")) return Promise.resolve(json(ADMIN));
    if (u.includes("/v1/setup/status")) {
      return Promise.resolve(
        json({ checks: [{ name: "media_server", ok: false, hint: "Emby refused the token." }] }),
      );
    }
    if (u.includes("/v1/setup/test")) return Promise.resolve(json({ ok: true }));
    if (u.includes("/v1/settings") && String(init?.method) === "PATCH") {
      return Promise.resolve(json({ results: [] }));
    }
    if (u.includes("/v1/settings")) return Promise.resolve(json({ features: {}, settings: SETTINGS }));
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
  return mock;
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

afterEach(() => vi.restoreAllMocks());

describe("Settings", () => {
  it("self-diagnoses each connection on its own block (§5 status-per-block)", async () => {
    stubFetch();
    renderAt("/settings/connections");
    // media_server's check fails, so its ConnectionBlock opens and shows the BE's hint
    // inline — diagnosis on the thing that fixes it, not in a separate checklist above.
    expect(await screen.findByText("Emby refused the token.")).toBeInTheDocument();
    // No standalone "connection checklist" duplicating the block statuses — the wiring
    // actions self-report on their own blocks, quiet once set up (§5, §13).
    expect(screen.queryByRole("heading", { name: /connection checklist/i })).not.toBeInTheDocument();
  });

  it("saves the whole page from one bar, sending only what changed", async () => {
    const fetchMock = stubFetch();
    renderAt("/settings/connections");

    // The bar is absent until something is dirty — a page being read stays quiet.
    expect(screen.queryByRole("region", { name: /unsaved changes/i })).not.toBeInTheDocument();

    await userEvent.type(await screen.findByLabelText("Library URL"), "9");
    const bar = await screen.findByRole("region", { name: /unsaved changes/i });
    expect(bar).toHaveTextContent("1 unsaved change");

    await userEvent.click(screen.getByRole("button", { name: /save changes/i }));
    const patch = fetchMock.mock.calls.find(
      ([u, i]) => String(u).includes("/v1/settings") && String(i?.method) === "PATCH",
    );
    const body = JSON.parse(String(patch?.[1]?.body));
    // Only the edited key — an untouched secret must not be sent, or it would be cleared (§9).
    expect(Object.keys(body.edits)).toEqual(["library.url"]);
    expect(body.edits["library.url"]).toBe("http://emby:80969");
  });

  it("discards edits without saving", async () => {
    stubFetch();
    renderAt("/settings/connections");

    await userEvent.type(await screen.findByLabelText("Library URL"), "9");
    await userEvent.click(screen.getByRole("button", { name: /discard/i }));
    expect(screen.queryByRole("region", { name: /unsaved changes/i })).not.toBeInTheDocument();
  });

  it("runs a per-block connection test", async () => {
    const fetchMock = stubFetch();
    renderAt("/settings/connections");

    const tests = await screen.findAllByRole("button", { name: /test connection/i });
    await userEvent.click(tests[0] as HTMLElement);
    expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/v1/setup/test"))).toBe(true);
  });

  // Regression: /v1/setup/test evaluates PERSISTED settings, so testing an UNSAVED edit
  // probes the OLD stored value — typing an Emby token then pressing Test 401'd against the
  // empty stored token, even though the right value was on screen. Test must PATCH the dirty
  // edits FIRST, then test. Asserted by call ORDER: the PATCH must precede the /setup/test.
  it("saves a dirty edit before testing, so Test checks what's on screen", async () => {
    const fetchMock = stubFetch();
    renderAt("/settings/connections");

    // Type into the media-server block (its Test button is the first), then Test WITHOUT Save.
    await userEvent.type(await screen.findByLabelText("Library URL"), "9");
    const tests = await screen.findAllByRole("button", { name: /test connection/i });
    await userEvent.click(tests[0] as HTMLElement);

    const order = fetchMock.mock.calls
      .map(([u, i], idx) => ({ idx, u: String(u), method: String((i as RequestInit)?.method) }))
      .filter(
        (c) => (c.u.includes("/v1/settings") && c.method === "PATCH") || c.u.includes("/v1/setup/test"),
      );
    const patchIdx = order.find((c) => c.u.includes("/v1/settings"))?.idx ?? -1;
    const testIdx = order.find((c) => c.u.includes("/v1/setup/test"))?.idx ?? -1;
    expect(patchIdx, "a dirty edit must be saved before testing").toBeGreaterThanOrEqual(0);
    expect(testIdx, "the test must still run").toBeGreaterThanOrEqual(0);
    expect(patchIdx).toBeLessThan(testIdx);
  });

  it("locks an env-pinned key on its page", async () => {
    stubFetch();
    renderAt("/settings/all");
    expect(await screen.findByLabelText("Job workers")).toBeDisabled();
  });
});

// V9's central claim: "dirty state survives tab switches".
//
// ⚠ This was BROKEN before V9 and invisible: `SettingsPage` held `edits` in its own useState,
// so navigating to another tab unmounted the page and discarded them with no warning. An
// operator editing a connection, stepping over to check a default, and coming back found their
// work gone. The buffer now lives in the layout, above the <Outlet />.
describe("Settings — the save bar spans tabs (V9)", () => {
  it("keeps an unsaved edit when you switch tabs and come back", async () => {
    stubFetch();
    renderAt("/settings/connections");

    const url = await screen.findByLabelText("Library URL");
    await userEvent.clear(url);
    await userEvent.type(url, "http://emby:9999");
    // The bar counts it, which is what tells the operator anything is staged at all.
    expect(await screen.findByText(/1 unsaved/i)).toBeInTheDocument();

    // Leave for another tab and come back — the tab bar is navigation, not a commit boundary.
    await userEvent.click(screen.getByRole("link", { name: "All settings" }));
    await screen.findByLabelText("Job workers");
    await userEvent.click(screen.getByRole("link", { name: "Connections" }));

    expect(await screen.findByLabelText("Library URL")).toHaveValue("http://emby:9999");
    expect(screen.getByText(/1 unsaved/i)).toBeInTheDocument();
  });

  // The count is global BY DESIGN. Per-tab buffers would make "2 unsaved" mean something
  // different depending on where you were standing, which is worse than no count.
  it("counts edits made on different tabs together", async () => {
    stubFetch();
    renderAt("/settings/connections");

    const url = await screen.findByLabelText("Library URL");
    await userEvent.clear(url);
    await userEvent.type(url, "http://emby:9999");

    await userEvent.click(screen.getByRole("link", { name: "All settings" }));
    const workers = await screen.findByLabelText("Job workers");
    // `job.workers` is env-pinned in the fixture, so edit the OTHER connection key instead —
    // asserting against a disabled field would prove nothing about the buffer.
    expect(workers).toBeDisabled();

    await userEvent.click(screen.getByRole("link", { name: "Connections" }));
    const tunarr = await screen.findByLabelText("Tunarr URL");
    await userEvent.clear(tunarr);
    await userEvent.type(tunarr, "http://tunarr:9999");

    expect(await screen.findByText(/2 unsaved/i)).toBeInTheDocument();
  });
});

// A pull that fails must SAY so. Before this, an "error" frame cleared the progress and
// refreshed — indistinguishable from success, leaving the operator to eventually notice
// the row still said "Download". The frame carries the reason; it belongs on screen.
describe("AI model pull", () => {
  it("surfaces a failed download instead of silently clearing it", async () => {
    let emit: ((e: MessageEvent) => void) | undefined;
    const mock = vi.fn((url: string) => {
      const u = String(url);
      if (u.includes("/v1/auth/me")) return Promise.resolve(json(ADMIN));
      if (u.includes("/v1/system/llm")) {
        return Promise.resolve(
          json({
            local: true,
            reachable: true,
            provider: "ollama",
            model: "",
            catalog: [
              {
                tag: "qwen3:8b",
                label: "Qwen3 8B",
                approxVramGiB: 5,
                fit: "fits",
                pulled: false,
                recommended: true,
                runtimeOk: true,
                why: "Good fit.",
              },
            ],
            hosted: [],
          }),
        );
      }
      if (u.includes("/v1/settings")) return Promise.resolve(json({ features: {}, settings: SETTINGS }));
      return Promise.resolve(json({}));
    });
    vi.stubGlobal("fetch", mock);
    vi.stubGlobal(
      "EventSource",
      class {
        addEventListener(type: string, cb: (e: MessageEvent) => void) {
          if (type === "llm_pull") emit = cb;
        }
        close() {}
      },
    );

    renderAt("/settings/ai");
    await userEvent.click(await screen.findByRole("button", { name: /download/i }));

    // The BE reports failure with a reason and percent -1 (a sentinel, not a percentage).
    emit?.({
      data: JSON.stringify({
        model: "qwen3:8b",
        status: "error",
        percent: -1,
        error: "no space left on device",
      }),
    } as MessageEvent);

    expect(await screen.findByText(/no space left on device/i)).toBeInTheDocument();
  });
});

// Each Settings page mounts its footer panel. These are one-line wirings in the route
// files that a component test can't see: both the model picker and the secrets panel were
// imported-but-never-rendered, so the feature was absent while every unit test stayed
// green. Asserting the panel reaches the page is what the component tests can't do.
describe("Settings page footers", () => {
  it("mounts the secrets panel on Security", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string) => {
        const u = String(url);
        if (u.includes("/v1/auth/me")) return Promise.resolve(json(ADMIN));
        if (u.includes("/v1/settings")) return Promise.resolve(json({ features: {}, settings: SETTINGS }));
        return Promise.resolve(json({}));
      }),
    );

    renderAt("/settings/security");
    // The three generated secrets are a closed set held in the component (config-design
    // §4), not a fetched list — so the assertion is that the panel is on the page at all.
    expect(await screen.findByText(/API token/i)).toBeInTheDocument();
    expect(screen.getByText(/Session secret/i)).toBeInTheDocument();
  });
});

// Wiring (Tunarr → the guide; Tunarr → the library) is no longer a manual button on
// Connections: it's an idempotent effect the server runs on save (config-design §5). So the
// Connections page must NOT show wiring actions, and saving a connection must just PATCH
// settings — the BE auto-wires (its own test proves the connectors fire). This guards that
// the confusing manual scaffolding stayed gone and didn't creep back.
describe("Connections auto-wires on save (no manual wiring UI)", () => {
  const stubWiring = () => {
    const mock = vi.fn((url: string, init?: RequestInit) => {
      const u = String(url);
      if (u.includes("/v1/auth/me")) return Promise.resolve(json(ADMIN));
      if (u.includes("/v1/setup/status")) {
        return Promise.resolve(
          json({
            checks: [
              { name: "livetv", ok: false, hint: "Tunarr is not a tuner yet." },
              { name: "tunarr_library", ok: false, hint: "Tunarr has no media source." },
            ],
          }),
        );
      }
      if (u.includes("/v1/settings") && String(init?.method) === "PATCH") {
        return Promise.resolve(json({ results: [] }));
      }
      if (u.includes("/v1/settings")) return Promise.resolve(json({ features: {}, settings: SETTINGS }));
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
    return mock;
  };

  it("shows no manual wiring buttons on Connections", async () => {
    stubWiring();
    renderAt("/settings/connections");
    // Wait for the page to settle (the Tunarr block renders).
    await screen.findByLabelText("Library URL");
    expect(screen.queryByRole("button", { name: /connect tunarr to the guide/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /wire tunarr to your library/i })).not.toBeInTheDocument();
  });

  it("saving a connection PATCHes settings and never calls a connect endpoint from the FE", async () => {
    const fetchMock = stubWiring();
    renderAt("/settings/connections");

    await userEvent.type(await screen.findByLabelText("Library URL"), "9");
    await userEvent.click(await screen.findByRole("button", { name: /save changes/i }));

    // The FE only saves — the server does the wiring. No /v1/setup/*-connect from here.
    const patched = fetchMock.mock.calls.find(
      ([u, i]) => String(u).includes("/v1/settings") && String(i?.method) === "PATCH",
    );
    expect(patched, "saving must PATCH settings").toBeTruthy();
    const connectCall = fetchMock.mock.calls.find(([u]) => String(u).includes("/v1/setup/tunarr-connect"));
    expect(connectCall, "the FE must not wire directly — the BE auto-wires on save").toBeFalsy();
  });
});
