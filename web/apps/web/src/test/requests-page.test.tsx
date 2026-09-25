import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ADMIN, failed, journey, MEMBER, proposal, renderAt, stub, title } from "@/test/requests-harness";

// #1405 — the Requests page's entry points, driven through the real router the way the Dashboard
// cards and a bookmark reach it. The lists, the detail and the nav badge have colocated specs
// under queue/.

afterEach(() => vi.restoreAllMocks());

describe("Dashboard cards link to the tab they name", () => {
  it("'Needs you' lands on Needs you and 'Acquiring' on In progress", async () => {
    stub({ me: ADMIN, proposals: [proposal] });
    const router = renderAt("/dashboard");

    await userEvent.click(await screen.findByRole("link", { name: /Acquiring/ }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/requests/in-progress"));

    await router.navigate({ to: "/dashboard" });
    await userEvent.click(await screen.findByRole("link", { name: /Needs you/ }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/requests/needs-you"));
  });

  it("'Acquiring' counts only what is in flight, not landed or given-up titles", async () => {
    stub({
      me: ADMIN,
      titles: {
        wanted: [title("a", "wanted")],
        requested: [title("b", "requested")],
        downloading: [title("c", "downloading")],
        available: [title("d", "available")],
        unavailable: [title("e", "unavailable", { lastError: "deadline exceeded" })],
      },
    });
    renderAt("/dashboard");

    const card = await screen.findByRole("link", { name: /Acquiring/ });
    await waitFor(() => expect(within(card).getByText("3")).toBeInTheDocument());
  });
});

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
