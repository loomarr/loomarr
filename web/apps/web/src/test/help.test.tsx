import { getGetDocMockHandler, getListDocsMockHandler, getMeMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

const TROUBLESHOOTING = `# Troubleshooting

Every red check deep-links here.

## Tunarr

Tunarr owns playout.

## LiveTV

One-time wiring.
`;

const DOCS = {
  quickstart: { slug: "quickstart", title: "Quickstart", markdown: "# Quickstart\n\nStart here." },
  troubleshooting: { slug: "troubleshooting", title: "Troubleshooting", markdown: TROUBLESHOOTING },
} as const;

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
      ],
    }),
    getGetDocMockHandler(({ params }) => {
      const doc = DOCS[params.slug as keyof typeof DOCS];
      if (!doc) throw new Error(`test asked for an unmodelled doc: ${String(params.slug)}`);
      return doc;
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

  it("filters the page list", async () => {
    stubHelp();
    renderAt("/help");
    await screen.findByRole("button", { name: "Troubleshooting" });

    await userEvent.type(screen.getByLabelText("Search"), "quick");
    expect(screen.getByRole("button", { name: "Quickstart" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Troubleshooting" })).not.toBeInTheDocument();
  });

  it("switches pages from the nav", async () => {
    stubHelp();
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
    stubHelp();
    renderAt("/help");
    await screen.findByRole("button", { name: "Troubleshooting" });

    await userEvent.click(screen.getByRole("button", { name: /search/i }));
    expect(await screen.findByRole("dialog", { name: /search loomarr/i })).toBeInTheDocument();
  });

  it("opens and closes on the keyboard shortcut", async () => {
    stubHelp();
    renderAt("/help");
    await screen.findByRole("button", { name: "Troubleshooting" });

    await userEvent.keyboard("{Meta>}k{/Meta}");
    expect(await screen.findByRole("dialog", { name: /search loomarr/i })).toBeInTheDocument();

    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: /search loomarr/i })).not.toBeInTheDocument();
  });
});
