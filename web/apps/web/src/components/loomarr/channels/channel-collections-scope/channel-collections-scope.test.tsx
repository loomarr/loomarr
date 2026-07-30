import type { ChannelPolicy } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { ChannelCollectionsScope } from "./channel-collections-scope";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <TooltipProvider>{children}</TooltipProvider>
    </QueryClientProvider>
  );
};

const render = (ui: ReactElement) => rtlRender(ui, { wrapper: makeWrapper() });

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stubCollections = (status: number, body: unknown) => {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      if (typeof url === "string" && url.includes("/v1/library/collections")) {
        return Promise.resolve(jsonResponse(status, body));
      }
      return Promise.resolve(jsonResponse(200, {}));
    }),
  );
};

afterEach(() => vi.unstubAllGlobals());

const TWO = {
  collections: [
    { id: "bs-1", name: "Halloween", childCount: 12 },
    { id: "bs-2", name: "Criterion" },
  ],
};

const EMPTY_SCOPE: ChannelPolicy = { ordering: "shuffle", scope: {} };

describe("ChannelCollectionsScope", () => {
  it("shows nothing picked, and no chips, for an empty scope", async () => {
    stubCollections(200, TWO);
    render(<ChannelCollectionsScope policy={EMPTY_SCOPE} onChange={vi.fn()} />);

    expect(await screen.findByRole("button", { name: /Add a collection/ })).toBeInTheDocument();
    expect(screen.queryByRole("listitem")).not.toBeInTheDocument();
  });

  it("picking one commits its opaque id, not its name", async () => {
    stubCollections(200, TWO);
    const onChange = vi.fn();
    render(<ChannelCollectionsScope policy={EMPTY_SCOPE} onChange={onChange} />);

    await userEvent.click(await screen.findByRole("button", { name: /Add a collection/ }));
    await userEvent.type(screen.getByLabelText("Search"), "hallo");
    await userEvent.click(await screen.findByText("Halloween"));

    // ⚠ The engine filters on ids, so committing "Halloween" would match nothing and the
    // channel would silently empty.
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ scope: expect.objectContaining({ collections: ["bs-1"] }) }),
    );
  });

  it("renders a picked collection as a named, removable chip", async () => {
    stubCollections(200, TWO);
    const onChange = vi.fn();
    render(
      <ChannelCollectionsScope
        policy={{ ...EMPTY_SCOPE, scope: { collections: ["bs-2"] } }}
        onChange={onChange}
      />,
    );

    // The chip shows the NAME even though the policy stores the id — the whole reason the
    // component holds the fetched list rather than rendering ids raw.
    expect(await screen.findByText("Criterion")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Remove Criterion" }));

    // ⚠ Removing the last one must send [], not undefined. `collections` is omitempty, so
    // dropping the key leaves the previous restriction in place and the field would appear to
    // clear while the channel stayed filtered (the same trap as scope.series).
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ scope: expect.objectContaining({ collections: [] }) }),
    );
  });

  // ⚠ **The reason this control is a type-ahead at all.** A real Emby returns 125 collections,
  // 112 of them auto-generated franchise groupings. A curated list must not be buried under
  // them, so curated sorts first — and this fails if the sort is dropped.
  it("ranks curated lists above auto-generated franchise collections", async () => {
    stubCollections(200, {
      collections: [
        { id: "f-1", name: "Alien Collection" },
        { id: "f-2", name: "Batman Collection" },
        { id: "c-1", name: "Zzz Best Of" }, // alphabetically last, but curated
      ],
    });
    render(<ChannelCollectionsScope policy={EMPTY_SCOPE} onChange={vi.fn()} />);

    // No query: browsing the whole list is exactly when ranking matters most. (Typing a term
    // would filter first and could exclude the curated row, testing the filter rather than the
    // sort — which is how the first version of this test fooled itself.)
    await userEvent.click(await screen.findByRole("button", { name: /Add a collection/ }));

    // Scope to the results list: plain findAllByText also matches the trigger button's label.
    const options = await screen.findAllByRole("option");
    expect(options).toHaveLength(3);
    expect(options[0]).toHaveTextContent("Zzz Best Of");
    // …and the franchises follow, alphabetically among themselves.
    expect(options[1]).toHaveTextContent("Alien Collection");
  });

  // ⚠ Escape closes the picker. SearchCommand deliberately does not bind it (the ⌘K palette
  // binds Escape window-level and would close twice), so the consumer owns the key — and its
  // comment's claim that a "Cancel affordance" covers this is what left the gap: a Cancel
  // BUTTON is not Escape, and a keyboard user reaches for the key first.
  it("closes the picker on Escape, discarding the query", async () => {
    stubCollections(200, TWO);
    render(<ChannelCollectionsScope policy={EMPTY_SCOPE} onChange={vi.fn()} />);

    await userEvent.click(await screen.findByRole("button", { name: /Add a collection/ }));
    await userEvent.type(screen.getByLabelText("Search"), "hallo");
    await userEvent.keyboard("{Escape}");

    // Back to the trigger, and reopening starts clean rather than restoring the old query.
    expect(await screen.findByRole("button", { name: /Add a collection/ })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Add a collection/ }));
    expect(screen.getByLabelText("Search")).toHaveValue("");
  });

  // Already-picked collections must not be offered again — picking a duplicate would write the
  // same id twice into the policy.
  it("does not offer a collection that is already picked", async () => {
    stubCollections(200, TWO);
    render(
      <ChannelCollectionsScope
        policy={{ ...EMPTY_SCOPE, scope: { collections: ["bs-1"] } }}
        onChange={vi.fn()}
      />,
    );

    await userEvent.click(await screen.findByRole("button", { name: /Add a collection/ }));
    await userEvent.type(screen.getByLabelText("Search"), "hallo");
    // The chip says "Halloween"; the results list must not add a second one.
    await waitFor(() => expect(screen.getAllByText("Halloween")).toHaveLength(1));
  });

  // ⚠ **Regression guard for the layout bug this component shipped with.** An `sr-only`
  // <legend> is `position:absolute`, and an absolutely-positioned legend escapes its fieldset's
  // containing block: it laid out below the fold, dragged `documentElement.scrollHeight` past
  // the viewport, and made the WHOLE WINDOW scroll behind a fixed sidebar. The accessible name
  // must come from `aria-label` on the fieldset instead.
  it("names the group without an escaping sr-only legend", async () => {
    stubCollections(200, TWO);
    const { container } = render(<ChannelCollectionsScope policy={EMPTY_SCOPE} onChange={vi.fn()} />);

    expect(await screen.findByRole("group", { name: "Only these collections" })).toBeInTheDocument();
    expect(container.querySelector("legend")).toBeNull();
  });

  // "You have none" and "the feature is unavailable" are different answers, and an empty box
  // reads as a broken control rather than an explanation.
  it("explains an empty collection list rather than showing nothing", async () => {
    stubCollections(200, { collections: [] });
    render(<ChannelCollectionsScope policy={EMPTY_SCOPE} onChange={vi.fn()} />);

    expect(await screen.findByText(/no collections yet/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Add a collection/ })).not.toBeInTheDocument();
  });

  // ⚠ A failing media server must not read as "you have none" — that sends the operator off to
  // create a collection they may already have.
  it("distinguishes a load failure from having no collections", async () => {
    stubCollections(502, { title: "Couldn't load collections" });
    render(<ChannelCollectionsScope policy={EMPTY_SCOPE} onChange={vi.fn()} />);

    expect(await screen.findByText(/Check the media server connection/i)).toBeInTheDocument();
    expect(screen.queryByText(/no collections yet/i)).not.toBeInTheDocument();
  });

  // 501 = no library configured. The control is not "empty", it is inapplicable — offering it
  // would advertise a narrowing that cannot be satisfied, and Connections is where that is fixed.
  it("renders nothing at all when no media library is configured", async () => {
    stubCollections(501, { title: "Collections aren't available" });
    const { container } = render(<ChannelCollectionsScope policy={EMPTY_SCOPE} onChange={vi.fn()} />);

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});
