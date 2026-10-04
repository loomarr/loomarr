import { LoomarrProvider } from "@loomarr/design-system";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { PhoneBottomBar } from "./phone-bottom-bar";

// LoomarrProvider as main.tsx mounts it: TabBar/BottomSheet are design-system (Tamagui) views.
const renderBar = (props: Partial<Parameters<typeof PhoneBottomBar>[0]> = {}, initialPath = "/guide") =>
  render(
    <LoomarrProvider theme="dark">
      <RouterHarness
        content={
          <PhoneBottomBar
            isAdmin={props.isAdmin ?? true}
            badges={props.badges}
            watchChannelId={props.watchChannelId}
          />
        }
        initialPath={initialPath}
      />
    </LoomarrProvider>,
  );

describe("PhoneBottomBar", () => {
  it("gives an admin with a channel Home/Watch/Guide/Requests plus More", async () => {
    renderBar({ isAdmin: true, watchChannelId: "ch-7" });
    const bar = await screen.findByRole("tablist", { name: "Primary navigation" });
    const tabs = within(bar)
      .getAllByRole("tab")
      .map((tab) => tab.getAttribute("aria-label"));
    expect(tabs).toEqual(["Home", "Watch", "Guide", "Requests"]);
    // More sits OUTSIDE the tablist (aria-required-children forbids a role="button" tab child),
    // so it's looked up against the whole document rather than `within(bar)`.
    expect(screen.getByRole("button", { name: "More" })).toBeInTheDocument();
  });

  it("drops Watch's slot (not a gap, not greyed) when no channel exists — X2", async () => {
    renderBar({ isAdmin: false, watchChannelId: undefined });
    const bar = await screen.findByRole("tablist", { name: "Primary navigation" });
    const tabs = within(bar)
      .getAllByRole("tab")
      .map((tab) => tab.getAttribute("aria-label"));
    expect(tabs).toEqual(["Home", "Guide", "Requests"]);
  });

  it("opens More to exactly the admin overflow, and selecting a row navigates and closes it", async () => {
    const user = userEvent.setup();
    renderBar({ isAdmin: true });
    await user.click(await screen.findByRole("button", { name: "More" }));
    const dialog = await screen.findByRole("dialog", { name: "More" });
    expect(
      within(dialog)
        .getAllByRole("button")
        .map((b) => b.getAttribute("aria-label")),
    ).toEqual(["Close", "Filler", "People", "Settings", "Help"]);
    await user.click(within(dialog).getByRole("button", { name: "Settings" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "More" })).not.toBeInTheDocument());
  });

  it("gives a member exactly Notifications and Help in More", async () => {
    const user = userEvent.setup();
    renderBar({ isAdmin: false });
    await user.click(await screen.findByRole("button", { name: "More" }));
    const dialog = await screen.findByRole("dialog", { name: "More" });
    expect(
      within(dialog)
        .getAllByRole("button")
        .map((b) => b.getAttribute("aria-label")),
    ).toEqual(["Close", "Notifications", "Help"]);
  });

  it("closes on Escape and returns focus to the More button", async () => {
    const user = userEvent.setup();
    renderBar({ isAdmin: true });
    const more = await screen.findByRole("button", { name: "More" });
    await user.click(more);
    await screen.findByRole("dialog", { name: "More" });
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "More" })).not.toBeInTheDocument());
    expect(more).toHaveFocus();
  });

  it("closes on a backdrop click and returns focus to the More button", async () => {
    const user = userEvent.setup();
    renderBar({ isAdmin: true });
    const more = await screen.findByRole("button", { name: "More" });
    await user.click(more);
    await screen.findByRole("dialog", { name: "More" });
    await user.click(screen.getByTestId("phone-bottom-bar-backdrop"));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "More" })).not.toBeInTheDocument());
    expect(more).toHaveFocus();
  });

  it("closes on its own × and returns focus to the More button", async () => {
    const user = userEvent.setup();
    renderBar({ isAdmin: true });
    const more = await screen.findByRole("button", { name: "More" });
    await user.click(more);
    const dialog = await screen.findByRole("dialog", { name: "More" });
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "More" })).not.toBeInTheDocument());
    expect(more).toHaveFocus();
  });

  it("marks aria-expanded on More while the sheet is open", async () => {
    const user = userEvent.setup();
    renderBar({ isAdmin: true });
    const more = await screen.findByRole("button", { name: "More" });
    expect(more).toHaveAttribute("aria-expanded", "false");
    await user.click(more);
    expect(more).toHaveAttribute("aria-expanded", "true");
  });

  it("selects the tab matching the current route, and no tab for an overflow page", async () => {
    renderBar({ isAdmin: true, watchChannelId: "ch-7" }, "/guide");
    expect(await screen.findByRole("tab", { name: "Guide" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Home" })).toHaveAttribute("aria-selected", "false");
  });

  it("selects Watch for any /channels/:id/watch route, not only the bar's own target", async () => {
    // "$id" here is itself the literal param VALUE the harness's route pattern resolves to —
    // a different channel than the bar's own target ("ch-7"), which is the point of the test.
    renderBar({ isAdmin: true, watchChannelId: "ch-7" }, "/channels/$id/watch");
    expect(await screen.findByRole("tab", { name: "Watch" })).toHaveAttribute("aria-selected", "true");
  });

  it("reaches Home before More in Tab order, matching visual left-to-right order", async () => {
    const user = userEvent.setup();
    renderBar({ isAdmin: true, watchChannelId: "ch-7" });
    await screen.findByRole("tab", { name: "Home" });
    await user.tab();
    expect(screen.getByRole("tab", { name: "Home" })).toHaveFocus();
    await user.tab(); // Watch
    await user.tab(); // Guide
    await user.tab(); // Requests
    await user.tab(); // More
    expect(screen.getByRole("button", { name: "More" })).toHaveFocus();
  });
});
