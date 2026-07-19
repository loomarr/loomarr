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
