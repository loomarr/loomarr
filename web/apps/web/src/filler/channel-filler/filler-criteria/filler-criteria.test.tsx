import { getListTaxonomyMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
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
  it("says which era a blank field is following, rather than looking like 'any'", () => {
    renderCriteria(<FillerCriteria selection={{}} onChange={vi.fn()} scopeEra={SCOPE} />);
    // ⚠ The typographic apostrophe (’), because the component renders `&rsquo;`. A straight quote
    // here fails on a string a human reading the screen would call identical.
    expect(screen.getByTestId("era-inherited")).toHaveTextContent("Following the channel’s era (1990–1999)");
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
