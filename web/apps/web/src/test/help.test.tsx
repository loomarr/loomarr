import { getGetDocMockHandler, getListDocsMockHandler, getMeMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";

const TROUBLESHOOTING = `# Troubleshooting

Every red check deep-links here.

## Tunarr

Tunarr owns playout.

## LiveTV

One-time wiring.
`;

const DOCS = {
  quickstart: {
    slug: "quickstart",
    section: "",
    title: "Quickstart",
    markdown: "# Quickstart\n\nStart here.",
  },
  troubleshooting: {
    slug: "troubleshooting",
    section: "",
    title: "Troubleshooting",
    markdown: TROUBLESHOOTING,
  },
  upgrade: { slug: "upgrade", section: "", title: "Upgrade", markdown: "# Upgrade\n\nPull the new image." },
} as const;

// The server's redirect table, for the one pre-restructure link the tests follow: the old
// quickstart "Upgrading" section now lives on its own page.
const MOVED: Record<string, keyof typeof DOCS> = { "quickstart#upgrading": "upgrade" };

// ⚠ The old stub matched docs by URL SUBSTRING, which forced a specificity ordering it had to get
// right by hand: `/v1/docs/troubleshooting` was tested before `/v1/docs`, because `includes` has
// no notion of a more specific route. `getGetDocMockHandler` binds to `*/v1/docs/:slug`, so MSW
// parses the path and hands over `params.slug` — the list and the single-page reads can no longer
// shadow each other, and a doc fetched at a path the spec does not declare goes UNHANDLED rather
// than falling through to the list branch.
const stubHelp = () => {
  server.use(
    getMeMockHandler(me()),
    getListDocsMockHandler({
      docs: [
        { slug: "quickstart", title: "Quickstart" },
        { slug: "troubleshooting", title: "Troubleshooting" },
        { slug: "upgrade", title: "Upgrade" },
      ],
    }),
    getGetDocMockHandler(({ params, request }) => {
      const section = new URL(request.url).searchParams.get("section") ?? "";
      const moved = MOVED[`${String(params.slug)}#${section}`];
      if (moved) return DOCS[moved];
      const doc = DOCS[params.slug as keyof typeof DOCS];
      if (!doc) throw new Error(`test asked for an unmodelled doc: ${String(params.slug)}`);
      return { ...doc, section };
    }),
    ...appHandlers(),
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
  return router;
};

describe("Help page", () => {
  // Opening Help with nothing selected reads as broken, so it defaults to the first page.
  //
  // ⚠ KNOWN FLAKE, ~1 run in 4, and NOT a timeout — it fails at ~1.2s against a 5s limit, so
  // raising the limit does nothing (measured). It reproduces at 69d0fb6, before the /guide
  // route existed, so it is not caused by route-tree growth either. The failing render shows
  // no Quickstart content at all, which points at the two chained fetches (page list, then
  // that page's markdown) resolving in an order this component does not re-query from.
  // Diagnosing it properly means reading the Help page's loading logic, which is V15's
  // rebuild — filed rather than papered over with a longer timeout.
  it("renders the first page as markdown, not raw source", async () => {
    stubHelp();
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
    stubHelp();
    renderAt("/help?page=troubleshooting");
    const heading = await screen.findByRole("heading", { name: "Tunarr" });
    expect(heading).toHaveAttribute("id", "tunarr");
    expect(await screen.findByRole("heading", { name: "LiveTV" })).toHaveAttribute("id", "livetv");
  });

  // A pre-restructure link names a section that moved to another page (#1736). The server
  // resolves it, and the URL follows so the page that holds that content now is open.
  it("follows a moved section to the page that holds it now", async () => {
    stubHelp();
    const router = renderAt("/help?page=quickstart&section=upgrading");
    await waitFor(() => expect(router.state.location.search).toEqual({ page: "upgrade" }));
    // The moved section's answer renders first, then the route's own read of the new page renders
    // the same markdown again with fresh nodes, so a node found on the first render can be detached
    // by the time it is asserted. Query the settled DOM instead of holding a node.
    expect(await screen.findByRole("link", { name: "Upgrade", current: "page" })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("Pull the new image.")).toBeInTheDocument());
  });

  it("filters the page list", async () => {
    stubHelp();
    renderAt("/help");
    await screen.findByRole("link", { name: "Troubleshooting" });

    await userEvent.type(screen.getByLabelText("Search"), "quick");
    expect(screen.getByRole("link", { name: "Quickstart" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Troubleshooting" })).not.toBeInTheDocument();
  });

  it("switches pages from the nav, whose items are links with their own URL (#2751)", async () => {
    stubHelp();
    renderAt("/help");
    const link = await screen.findByRole("link", { name: "Troubleshooting" });
    expect(link).toHaveAttribute("href", expect.stringContaining("page=troubleshooting"));
    await userEvent.click(link);
    expect(await screen.findByRole("heading", { name: "Tunarr" })).toBeInTheDocument();
  });
});

describe("Command palette", () => {
  // The shell's Search button called a setter whose value was DISCARDED
  // (`const [, setCommandOpen] = useState(false)`), so clicking it did nothing at all.
  // Both entry points now drive one piece of state.
  it("opens from the shell's Search button", async () => {
    stubHelp();
    renderAt("/help");
    await screen.findByRole("link", { name: "Troubleshooting" });

    await userEvent.click(screen.getByRole("button", { name: /search/i }));
    expect(await screen.findByRole("dialog", { name: /search loomarr/i })).toBeInTheDocument();
  });

  // #2751: below `md` the rail (and its Search button) is gone; the phone header's Search icon,
  // top right, opens the same search full screen, and Close shuts it without a key or a backdrop.
  it("opens full-screen from the phone header's Search icon", async () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      addEventListener: vi.fn(),
      matches: false,
      media: query,
      removeEventListener: vi.fn(),
    }));
    try {
      stubHelp();
      renderAt("/help");

      // Exactly "Search": the rail's own button (CSS-hidden here, still in jsdom's tree) is named
      // "Search…" plus its shortcut.
      await userEvent.click(await screen.findByRole("button", { name: "Search" }));
      const dialog = await screen.findByRole("dialog", { name: /search loomarr/i });
      expect(dialog.style.inset).toBe("0px");

      await userEvent.click(within(dialog).getByRole("button", { name: "Close search" }));
      await waitFor(() =>
        expect(screen.queryByRole("dialog", { name: /search loomarr/i })).not.toBeInTheDocument(),
      );
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("opens and closes on the keyboard shortcut", async () => {
    stubHelp();
    renderAt("/help");
    await screen.findByRole("link", { name: "Troubleshooting" });

    await userEvent.keyboard("{Meta>}k{/Meta}");
    expect(await screen.findByRole("dialog", { name: /search loomarr/i })).toBeInTheDocument();

    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: /search loomarr/i })).not.toBeInTheDocument();
  });
});
