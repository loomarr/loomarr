import { getListTaxonomyMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactElement, useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { FillerCriteria } from "./filler-criteria";

// ⚠ MSW, not `vi.stubGlobal("fetch")` (retired-ok). The sibling `channel-filler.test.tsx` still
// hand-rolls a fetch stub — it predates the V53e migration and is on the list — but a NEW file
// adding another private encoding of what the wire looks like is the exact thing that migration
// is removing. The handler is orval-generated, so a renamed route breaks it loudly instead of
// silently ceasing to match. The server lifecycle is installed globally in `test/setup.ts`.

beforeEach(() => {
  // The panel fetches the product taxonomy for its category chips (§10 V45a). These tests are
  // about the ERA field, so the vocabulary just has to exist.
  server.use(getListTaxonomyMockHandler());
});

// The panel calls useProductCategories (a live generated-API hook), so it needs a QueryClient
// even in isolation. No router: nothing here is a TanStack Link.
const renderCriteria = (ui: ReactElement) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
};

const CriteriaHarness = ({
  initial,
  onChange,
  programmingDates,
}: {
  initial: Parameters<typeof FillerCriteria>[0]["selection"];
  onChange: (selection: Parameters<typeof FillerCriteria>[0]["selection"]) => void;
  programmingDates?: Parameters<typeof FillerCriteria>[0]["programmingDates"];
}) => {
  const [selection, setSelection] = useState(initial);
  return (
    <FillerCriteria
      selection={selection}
      programmingDates={programmingDates}
      onChange={(next) => {
        onChange(next);
        setSelection(next);
      }}
    />
  );
};

const SCOPE = { from: 1990, to: 1999 };

// The three states of a filler era (§10 V51f).
//
// ⚠ **Two of them used to be the same value, so one was unreachable.** The server applies
// `policy.scope.era` to an UNSET filler era, live on every derivation — and the UI rendered that
// as two blank inputs, which reads as "any era". So a channel quietly drawing 1990s ads looked
// like it was drawing from everything, and an operator who wanted the whole catalog had no way to
// say so: clearing the fields simply re-inherited. Presence is now the opt-in, and these tests
// assert the two escapes that make the third state reachable.
describe("FillerCriteria era", () => {
  it("keeps disjoint explicit windows and rejects an invalid edit", async () => {
    const onChange = vi.fn();
    renderCriteria(
      <CriteriaHarness initial={{ eraWindows: [{ from: 1990, to: 1999 }] }} onChange={onChange} />,
    );

    await userEvent.type(screen.getByLabelText("New date range from"), "2005");
    await userEvent.type(screen.getByLabelText("New date range to"), "2009");
    await userEvent.click(screen.getByRole("button", { name: "Add range" }));
    expect(onChange).toHaveBeenLastCalledWith({
      eraWindows: [
        { from: 1990, to: 1999 },
        { from: 2005, to: 2009 },
      ],
    });

    const from = screen.getAllByLabelText("From year")[0]!;
    await userEvent.clear(from);
    await userEvent.type(from, "2001");
    await userEvent.tab();
    expect(screen.getByRole("alert")).toHaveTextContent("From no later than To");
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it("commits a recovered date pair without widening the range after an invalid From edit", async () => {
    const onChange = vi.fn();
    renderCriteria(
      <CriteriaHarness
        initial={{
          eraWindows: [
            { from: 1990, to: 1999 },
            { from: 2011, to: 2014 },
          ],
        }}
        onChange={onChange}
      />,
    );

    const from = screen.getAllByLabelText("From year")[0]!;
    const to = screen.getAllByLabelText("To year")[0]!;
    await userEvent.clear(from);
    await userEvent.type(from, "2005");
    await userEvent.tab();

    expect(screen.getByRole("alert")).toHaveTextContent("From no later than To");
    expect(onChange).not.toHaveBeenCalled();

    await userEvent.clear(to);
    await userEvent.type(to, "2009");
    await userEvent.tab();

    expect(onChange).toHaveBeenLastCalledWith({
      eraWindows: [
        { from: 2005, to: 2009 },
        { from: 2011, to: 2014 },
      ],
    });
    expect(screen.getAllByLabelText("From year")[0]).toHaveValue(2005);
    expect(screen.getAllByLabelText("To year")[0]).toHaveValue(2009);
  });

  it("commits a recovered date pair after an invalid To edit", async () => {
    const onChange = vi.fn();
    renderCriteria(
      <CriteriaHarness initial={{ eraWindows: [{ from: 1990, to: 1999 }] }} onChange={onChange} />,
    );

    const from = screen.getAllByLabelText("From year")[0]!;
    const to = screen.getAllByLabelText("To year")[0]!;
    await userEvent.clear(to);
    await userEvent.type(to, "1985");
    await userEvent.tab();

    expect(screen.getByRole("alert")).toHaveTextContent("From no later than To");
    expect(onChange).not.toHaveBeenCalled();

    await userEvent.clear(from);
    await userEvent.type(from, "1980");
    await userEvent.tab();

    expect(onChange).toHaveBeenLastCalledWith({ eraWindows: [{ from: 1980, to: 1985 }] });
  });

  it("uses any and inheritance without retaining era windows", async () => {
    const onChange = vi.fn();
    renderCriteria(
      <CriteriaHarness
        initial={{ audience: "kids", eraWindows: [{ from: 1990, to: 1999 }] }}
        programmingDates={{
          movieRelease: [{ from: 1990, to: 1999 }],
          seriesAiring: [{ from: 2005, to: 2009 }],
        }}
        onChange={onChange}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Use any era" }));
    expect(onChange).toHaveBeenLastCalledWith({ audience: "kids", era: {} });
    await userEvent.click(screen.getByRole("button", { name: /Follow the channel’s era/ }));
    expect(onChange).toHaveBeenLastCalledWith({ audience: "kids" });
  });

  it("labels the movie-release and airing union, with premiere only as the fallback", () => {
    const { rerender } = renderCriteria(
      <FillerCriteria
        selection={{}}
        programmingDates={{
          movieRelease: [{ from: 1990, to: 1999 }],
          seriesPremiere: [{ from: 1980, to: 1989 }],
          seriesAiring: [{ from: 2005, to: 2009 }],
        }}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId("era-inherited")).toHaveTextContent("1990–1999, 2005–2009");

    rerender(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <FillerCriteria
          selection={{}}
          programmingDates={{
            movieRelease: [{ from: 1990, to: 1999 }],
            seriesPremiere: [{ from: 1980, to: 1989 }],
          }}
          onChange={vi.fn()}
        />
      </QueryClientProvider>,
    );
    expect(screen.getByTestId("era-inherited")).toHaveTextContent("1980–1999");
  });

  it("hides the range-adder once all eight windows are present", () => {
    renderCriteria(
      <FillerCriteria
        selection={{
          eraWindows: Array.from({ length: 8 }, (_, index) => ({
            from: 1900 + index * 10,
            to: 1905 + index * 10,
          })),
        }}
        onChange={vi.fn()}
      />,
    );
    expect(screen.queryByLabelText("New date range from")).not.toBeInTheDocument();
  });
  it("says which era a blank field is following, rather than looking like 'any'", () => {
    renderCriteria(<FillerCriteria selection={{}} onChange={vi.fn()} scopeEra={SCOPE} />);
    // ⚠ The typographic apostrophe (’), because the component renders `&rsquo;`. A straight quote
    // here fails on a string a human reading the screen would call identical.
    expect(screen.getByTestId("era-inherited")).toHaveTextContent("Following the channel’s era (1990–1999)");
  });

  it.each([
    ["From only", { from: 1990 }, "1990"],
    ["To only", { to: 1999 }, "1999"],
  ])("keeps a one-sided channel era visible for inheritance (%s)", async (_name, scopeEra, label) => {
    const onChange = vi.fn();
    renderCriteria(
      <FillerCriteria selection={{ audience: "kids" }} onChange={onChange} scopeEra={scopeEra} />,
    );

    expect(screen.getByTestId("era-inherited")).toHaveTextContent(`Following the channel’s era (${label})`);
    await userEvent.click(screen.getByRole("button", { name: "Use any era" }));
    expect(onChange).toHaveBeenCalledWith({ audience: "kids", era: {} });
  });

  // ⚠ An EMPTY range, not a removed key. Presence is what tells the server the operator ANSWERED
  // "any" — a cleared field is indistinguishable from never having touched it, which is the bug.
  it("'Use any era' sends a present-but-empty range", async () => {
    const onChange = vi.fn();
    renderCriteria(<FillerCriteria selection={{ audience: "kids" }} onChange={onChange} scopeEra={SCOPE} />);

    await userEvent.click(screen.getByRole("button", { name: "Use any era" }));

    expect(onChange).toHaveBeenCalledWith({ audience: "kids", era: {} });
  });

  // ...and the way back REMOVES the key, because absence is what "inherit" is.
  it("'Follow the channel's era' drops the key entirely", async () => {
    const onChange = vi.fn();
    renderCriteria(
      <FillerCriteria selection={{ audience: "kids", era: {} }} onChange={onChange} scopeEra={SCOPE} />,
    );

    await userEvent.click(screen.getByRole("button", { name: /Follow the channel’s era/ }));

    expect(onChange).toHaveBeenCalledWith({ audience: "kids" });
    // ⚠ ABSENT, not present-and-empty — `toHaveBeenCalledWith` alone would pass for either, and
    // the difference between them is the whole point of the three states.
    expect(onChange.mock.calls[0]?.[0]).not.toHaveProperty("era");
  });

  it("shows neither affordance once a real range is set", () => {
    renderCriteria(
      <FillerCriteria selection={{ era: { from: 1975, to: 1985 } }} onChange={vi.fn()} scopeEra={SCOPE} />,
    );
    expect(screen.queryByTestId("era-inherited")).not.toBeInTheDocument();
    expect(screen.queryByTestId("era-any")).not.toBeInTheDocument();
  });

  // A channel with no programming era has nothing to inherit, so there is nothing to explain and
  // no escape to offer — a blank field genuinely IS "any" there.
  it("says nothing about inheritance when the channel has no era", () => {
    renderCriteria(<FillerCriteria selection={{}} onChange={vi.fn()} />);
    expect(screen.queryByTestId("era-inherited")).not.toBeInTheDocument();
  });

  // ⚠ The `To` year reaches the caller. It was rendered, typed, canonicalised and
  // inverted-range-validated for several phases while every backend consumer read only `from`,
  // so a test that only proves `from` round-trips would have passed throughout the bug.
  it("commits the To year", async () => {
    const onChange = vi.fn();
    renderCriteria(
      <FillerCriteria selection={{ era: { from: 1990 } }} onChange={onChange} scopeEra={SCOPE} />,
    );

    const to = screen.getByLabelText("To year");
    await userEvent.type(to, "1999");
    await userEvent.tab();

    expect(onChange).toHaveBeenCalledWith({ era: { from: 1990, to: 1999 } });
  });
});

describe("FillerCriteria geography", () => {
  it("shows the inherited installation country and market", () => {
    renderCriteria(
      <FillerCriteria
        selection={{}}
        onChange={vi.fn()}
        installationGeography={{ country: "US", market: "New York" }}
      />,
    );
    expect(screen.getByTestId("geography-inherited")).toHaveTextContent(
      "Following this installation (US · New York)",
    );
  });

  it("creates an explicit channel override and can return to inheritance", async () => {
    const onChange = vi.fn();
    const { rerender } = renderCriteria(
      <FillerCriteria
        selection={{ audience: "kids" }}
        onChange={onChange}
        installationGeography={{ country: "US" }}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Set for this channel" }));
    expect(onChange).toHaveBeenCalledWith({ audience: "kids", geography: { country: "US" } });

    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <FillerCriteria
          selection={{ audience: "kids", geography: { country: "US", market: "New York" } }}
          onChange={onChange}
        />
      </QueryClientProvider>,
    );
    await userEvent.click(screen.getByRole("button", { name: "Follow installation geography" }));
    expect(onChange).toHaveBeenLastCalledWith({ audience: "kids" });
  });

  it("keeps an explicit Channel inside the Installation country", () => {
    renderCriteria(
      <FillerCriteria
        selection={{ geography: { country: "CA", market: "Toronto" } }}
        onChange={vi.fn()}
        installationGeography={{ country: "US" }}
      />,
    );
    const country = screen.getByLabelText("Country code");
    expect(country).toHaveValue("US");
    expect(country).toBeDisabled();
  });
});
