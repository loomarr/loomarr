import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { WatchPill } from "./watch-pill";

// ⚠ The HEALTH RULE is not tested here, because it is not here. It briefly lived in a
// `watchHealth()` helper beside this component and was moved to the server (`GET
// /v1/filler/watch`, §10 V38c): the sources listing is admin-only, so a client-side derivation
// left members with a permanently grey dot, and the rule itself is domain logic worth testing
// against the store rather than against a hand-built array of fake rows. `fillerwatch_test.go`
// owns those cases — all-sources-off, empty-catalog, never-fetched-is-not-stale, and the rest.
//
// What is left here is the component's own job: render what it is given, and say the dot's
// meaning in words.

describe("WatchPill", () => {
  it("renders the status line", () => {
    render(<WatchPill status="4 of 5 sources on · 9 clips" health="healthy" />);
    expect(screen.getByText("4 of 5 sources on · 9 clips")).toBeInTheDocument();
  });

  // ⚠ The dot is aria-hidden, so its meaning has to reach a screen reader as WORDS. A colour with
  // no text equivalent is exactly the failure axe cannot catch: it sees a decorated span, not a
  // missing sentence.
  it("says in words what the dot says in colour", () => {
    const { rerender } = render(<WatchPill status="x" health="healthy" />);
    expect(screen.getByText("Watching your sources")).toBeInTheDocument();

    rerender(<WatchPill status="x" health="attention" />);
    expect(screen.getByText("Filler needs attention")).toBeInTheDocument();

    rerender(<WatchPill status="x" health="unconfigured" />);
    expect(screen.getByText("No sources set up yet")).toBeInTheDocument();
  });
});
