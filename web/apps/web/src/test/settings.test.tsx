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
  it("puts the re-runnable checklist on Connections (§13 troubleshooting console)", async () => {
    stubFetch();
    renderAt("/settings/connections");
    expect(await screen.findByRole("heading", { name: /connection checklist/i })).toBeInTheDocument();
    // The hint appears once the check settles to `fail` — while the query is in
    // flight the row reads "running", which deliberately suppresses it.
    expect(await screen.findByText("Emby refused the token.")).toBeInTheDocument();
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

  it("locks an env-pinned key on its page", async () => {
    stubFetch();
    renderAt("/settings/advanced");
    expect(await screen.findByLabelText("Job workers")).toBeDisabled();
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
  it("mounts the secrets panel on Users & security", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string) => {
        const u = String(url);
        if (u.includes("/v1/auth/me")) return Promise.resolve(json(ADMIN));
        if (u.includes("/v1/settings")) return Promise.resolve(json({ features: {}, settings: SETTINGS }));
        return Promise.resolve(json({}));
      }),
    );

    renderAt("/settings/users");
    // The three generated secrets are a closed set held in the component (config-design
    // §4), not a fetched list — so the assertion is that the panel is on the page at all.
    expect(await screen.findByText(/API token/i)).toBeInTheDocument();
    expect(screen.getByText(/Session secret/i)).toBeInTheDocument();
  });
});
