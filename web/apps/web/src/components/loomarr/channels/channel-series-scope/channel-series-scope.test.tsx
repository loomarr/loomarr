import type { ChannelPolicy } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { ChannelSeriesScope } from "./channel-series-scope";

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

const stubSearch = (candidates: unknown[]) => {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      if (typeof url === "string" && url.includes("/v1/search")) {
        return Promise.resolve(jsonResponse(200, { candidates }));
      }
      return Promise.resolve(jsonResponse(200, {}));
    }),
  );
};

afterEach(() => vi.unstubAllGlobals());

const POPULATED: ChannelPolicy = {
  ordering: "shuffle",
  scope: { era: { from: 1990, to: 1999 } },
  applied: [{ kind: "blockMax", from: "8", to: "unbounded" }],
};

describe("ChannelSeriesScope", () => {
  it("shows nothing picked, and no chips, for an empty scope", () => {
    stubSearch([]);
    render(<ChannelSeriesScope policy={POPULATED} onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: /Add a show/ })).toBeInTheDocument();
    expect(screen.queryByRole("listitem")).not.toBeInTheDocument();
  });

  it("renders a chip per stored key", () => {
    stubSearch([]);
    render(
      <ChannelSeriesScope
        policy={{ scope: { series: ["series:tvdb:73739", "series:tvdb:71663"] } }}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(screen.getByText("tvdb 73739")).toBeInTheDocument();
  });

  // The whole reason this needed a picker rather than a text box: the field is
  // []provision.Key — RESOLVED ids, never names — so a search result must be lowered to a key.
  it("adds a picked series as a resolved provisioning key", async () => {
    stubSearch([{ name: "The Simpsons", year: 1989, mediaType: "series", tvdbId: 71663, inLibrary: true }]);
    const onChange = vi.fn();
    render(<ChannelSeriesScope policy={POPULATED} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: /Add a show/ }));
    await userEvent.type(screen.getByLabelText("Search"), "simpsons");
    await userEvent.click(await screen.findByText("The Simpsons"));

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        scope: expect.objectContaining({ series: ["series:tvdb:71663"], era: POPULATED.scope?.era }),
      }),
    );
    // Reconcile-owned `applied` rides through untouched.
    expect(onChange.mock.lastCall?.[0].applied).toBe(POPULATED.applied);
  });

  // A movie can never produce a valid `scope.series` entry, and a series with no resolvable id
  // would lower to a `name:` key the backend's ParseKey rejects. Both are filtered out rather
  // than offered-then-rejected — the same prevention the lineup editor uses.
  it("offers only series that carry a resolvable id", async () => {
    stubSearch([
      { name: "Heat", year: 1995, mediaType: "movie", tmdbId: 949, inLibrary: true },
      { name: "Nameless Show", mediaType: "series", inLibrary: false },
      { name: "The Simpsons", year: 1989, mediaType: "series", tvdbId: 71663, inLibrary: true },
    ]);
    render(<ChannelSeriesScope policy={POPULATED} onChange={vi.fn()} />);

    await userEvent.click(screen.getByRole("button", { name: /Add a show/ }));
    await userEvent.type(screen.getByLabelText("Search"), "the");

    await waitFor(() => expect(screen.getByText("The Simpsons")).toBeInTheDocument());
    expect(screen.queryByText("Heat")).not.toBeInTheDocument();
    expect(screen.queryByText("Nameless Show")).not.toBeInTheDocument();
  });

  it("does not offer a series already picked", async () => {
    stubSearch([{ name: "The Simpsons", year: 1989, mediaType: "series", tvdbId: 71663, inLibrary: true }]);
    render(<ChannelSeriesScope policy={{ scope: { series: ["series:tvdb:71663"] } }} onChange={vi.fn()} />);

    await userEvent.click(screen.getByRole("button", { name: /Add a show/ }));
    await userEvent.type(screen.getByLabelText("Search"), "simpsons");

    await waitFor(() => expect(screen.queryByText("The Simpsons")).not.toBeInTheDocument());
  });

  // Clearing the last chip must send [] rather than dropping the key: `series` is omitempty,
  // so an undefined would be dropped from the JSON and the previous restriction would survive
  // the merge — the field would appear to clear and keep filtering. Same trap as runtimeMax.
  it("removing the last series commits an empty array, not undefined", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(<ChannelSeriesScope policy={{ scope: { series: ["series:tvdb:71663"] } }} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: /Remove tvdb 71663/ }));

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ scope: { series: [] } }));
    expect(onChange.mock.lastCall?.[0].scope).toHaveProperty("series");
  });

  it("removes only the chosen series, keeping the rest", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(
      <ChannelSeriesScope
        policy={{ scope: { series: ["series:tvdb:73739", "series:tvdb:71663"] } }}
        onChange={onChange}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /Remove tvdb 73739/ }));

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ scope: { series: ["series:tvdb:71663"] } }),
    );
  });

  // ⚠ Escape closes the picker, the same as Cancel. SearchCommand does not bind it by default
  // (the ⌘K palette binds it window-level and would close twice), so this consumer opts in —
  // and this test is what stops the opt-in being dropped in a later refactor. The gap shipped
  // in all four consumers precisely because every test clicked Cancel instead.
  it("closes the picker on Escape, discarding the query", async () => {
    stubSearch([]);
    render(<ChannelSeriesScope policy={POPULATED} onChange={vi.fn()} />);

    await userEvent.click(screen.getByRole("button", { name: /Add a show/ }));
    await userEvent.type(screen.getByLabelText("Search"), "simp");
    await userEvent.keyboard("{Escape}");

    expect(await screen.findByRole("button", { name: /Add a show/ })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Add a show/ }));
    expect(screen.getByLabelText("Search")).toHaveValue("");
  });
});
