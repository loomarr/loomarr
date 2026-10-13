import { screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { failed, journey, MEMBER, renderAt, stub } from "@/test/requests-harness";

// #1405 — the Requests page's own entry points (its landing tab and first-request empty state).
// The lists, the detail and the nav badge have colocated specs under queue/.
//
// Home no longer repeats "requests need you" or a failed request on its own strip (#1822
// evidence: "Neutral channel inventory count") — that now has its own surface, the Requests nav
// badge, with "Your requests" or "On the way" still on Home itself. There is nothing left here to
// drive through Home's strip.

afterEach(() => vi.restoreAllMocks());

describe("Requests landing", () => {
  it("opens on Needs you when one of your requests failed", async () => {
    stub({ me: MEMBER, journeys: [failed("j-bad")] });
    const router = renderAt("/requests");
    await waitFor(() => expect(router.state.location.pathname).toBe("/requests/needs-you"));
  });

  it("opens on In progress when nothing is waiting on you", async () => {
    stub({ me: MEMBER, journeys: [journey("j-gen", { milestone: "generating" })] });
    const router = renderAt("/requests");
    await waitFor(() => expect(router.state.location.pathname).toBe("/requests/in-progress"));
  });

  it("offers the one next action when you have never requested a channel", async () => {
    stub({ me: MEMBER });
    renderAt("/requests/in-progress");
    expect(await screen.findByText("You haven't requested a channel yet")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Request a channel" })).toBeInTheDocument();
  });
});
