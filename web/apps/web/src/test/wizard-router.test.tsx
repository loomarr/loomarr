import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
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
    if (u.includes("/v1/settings")) {
      return Promise.resolve(
        json(
          {
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
    expect(await screen.findByRole("heading", { name: /create the owning admin/i })).toBeInTheDocument();
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

  it("renders every check as words and blocks while a required one is red", async () => {
    stubFetch({
      authed: true,
      setupCompleted: false,
      checks: [
        { name: "media_server", ok: true },
        { name: "tunarr", ok: false, hint: "Tunarr didn't answer on that URL." },
      ],
    });
    renderAt("/wizard");

    expect(await screen.findByText("Media server")).toBeInTheDocument();
    expect(screen.getByText("Tunarr")).toBeInTheDocument();
    // A failure is the BE's plain-language hint, never a stack trace (§13).
    expect(screen.getByText("Tunarr didn't answer on that URL.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();
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
