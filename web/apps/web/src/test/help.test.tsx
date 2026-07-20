import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };

const TROUBLESHOOTING = `# Troubleshooting

Every red check deep-links here.

## Tunarr

Tunarr owns playout.

## LiveTV

One-time wiring.
`;

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stubFetch = () => {
  const mock = vi.fn((url: string) => {
    const u = String(url);
    if (u.includes("/v1/auth/me")) return Promise.resolve(json(ADMIN));
    if (u.includes("/v1/docs/troubleshooting")) {
      return Promise.resolve(
        json({ slug: "troubleshooting", title: "Troubleshooting", markdown: TROUBLESHOOTING }),
      );
    }
    if (u.includes("/v1/docs/quickstart")) {
      return Promise.resolve(
        json({ slug: "quickstart", title: "Quickstart", markdown: "# Quickstart\n\nStart here." }),
      );
    }
    if (u.includes("/v1/docs")) {
      return Promise.resolve(
        json({
          docs: [
            { slug: "quickstart", title: "Quickstart" },
            { slug: "troubleshooting", title: "Troubleshooting" },
          ],
        }),
      );
    }
    if (u.includes("/v1/settings")) return Promise.resolve(json({ features: {}, settings: [] }));
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

describe("Help page", () => {
  // Opening Help with nothing selected reads as broken, so it defaults to the first page.
  it("renders the first page as markdown, not raw source", async () => {
    stubFetch();
    renderAt("/help");
    // A heading ELEMENT, not a literal "# Quickstart" string — the difference between
    // rendering markdown and printing it.
    expect(await screen.findByRole("heading", { name: "Quickstart", level: 2 })).toBeInTheDocument();
    expect(screen.queryByText(/^# Quickstart$/)).not.toBeInTheDocument();
    expect(screen.getByText("Start here.")).toBeInTheDocument();
  });

  // §13: "every red check deep-links to its section". The API emits
  // `troubleshooting#tunarr`, so the page must be addressable by slug AND the rendered
  // heading must carry the matching id — a Go test and a core test both pin the anchors,
  // and this pins that the RENDERER applies them.
  it("opens the page named in the URL and gives headings their anchor ids", async () => {
    stubFetch();
    renderAt("/help?page=troubleshooting");
    const heading = await screen.findByRole("heading", { name: "Tunarr" });
    expect(heading).toHaveAttribute("id", "tunarr");
    expect(await screen.findByRole("heading", { name: "LiveTV" })).toHaveAttribute("id", "livetv");
  });

  it("filters the page list", async () => {
    stubFetch();
    renderAt("/help");
    await screen.findByRole("button", { name: "Troubleshooting" });

    await userEvent.type(screen.getByLabelText("Search"), "quick");
    expect(screen.getByRole("button", { name: "Quickstart" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Troubleshooting" })).not.toBeInTheDocument();
  });

  it("switches pages from the nav", async () => {
    stubFetch();
    renderAt("/help");
    await userEvent.click(await screen.findByRole("button", { name: "Troubleshooting" }));
    expect(await screen.findByRole("heading", { name: "Tunarr" })).toBeInTheDocument();
  });
});

describe("Command palette", () => {
  // The shell's Search button called a setter whose value was DISCARDED
  // (`const [, setCommandOpen] = useState(false)`), so clicking it did nothing at all.
  // Both entry points now drive one piece of state.
  it("opens from the shell's Search button", async () => {
    stubFetch();
    renderAt("/help");
    await screen.findByRole("button", { name: "Troubleshooting" });

    await userEvent.click(screen.getByRole("button", { name: /search/i }));
    expect(await screen.findByRole("dialog", { name: /search loomarr/i })).toBeInTheDocument();
  });

  it("opens and closes on the keyboard shortcut", async () => {
    stubFetch();
    renderAt("/help");
    await screen.findByRole("button", { name: "Troubleshooting" });

    await userEvent.keyboard("{Meta>}k{/Meta}");
    expect(await screen.findByRole("dialog", { name: /search loomarr/i })).toBeInTheDocument();

    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: /search loomarr/i })).not.toBeInTheDocument();
  });
});
