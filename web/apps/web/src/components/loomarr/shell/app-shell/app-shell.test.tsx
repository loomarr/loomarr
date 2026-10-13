import { LoomarrProvider } from "@loomarr/design-system";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { AppShell } from "./app-shell";

const renderShell = (isAdmin: boolean) =>
  render(<RouterHarness content={<AppShell isAdmin={isAdmin}>content</AppShell>} />);

const renderShellAt = (initialPath: string, props: Partial<Parameters<typeof AppShell>[0]> = {}) =>
  render(
    <RouterHarness
      content={
        <AppShell isAdmin={props.isAdmin ?? true} watchChannelId={props.watchChannelId}>
          content
        </AppShell>
      }
      initialPath={initialPath}
    />,
  );

describe("AppShell", () => {
  it("offers a skip link as the first Tab stop, targeting the main content", async () => {
    renderShell(true);
    const main = await screen.findByRole("main");
    const skip = screen.getByRole("link", { name: "Skip to content" });

    expect(skip).toHaveAttribute("href", `#${main.id}`);
    expect(main).toHaveAttribute("tabindex", "-1");
    await userEvent.tab();
    expect(skip).toHaveFocus();
  });

  it("shows the server identity above the account footer and links admins to About", async () => {
    render(
      <RouterHarness
        content={
          <AppShell isAdmin serverVersion="v0.9.3 (modified)">
            content
          </AppShell>
        }
      />,
    );

    const version = await screen.findByRole("link", { name: "Loomarr v0.9.3 (modified) — About" });
    expect(version).toHaveAttribute("href", "/settings/system/about");
    expect(version).toHaveClass("text-static-400");
    expect(version).not.toHaveClass("text-static-500");
    expect(
      within(screen.getByRole("navigation", { name: "Primary" })).queryByRole("link", {
        name: /loomarr v0\.9\.3/i,
      }),
    ).not.toBeInTheDocument();
  });

  it("shows members the version without linking them into admin Settings", async () => {
    render(
      <RouterHarness
        content={
          <AppShell isAdmin={false} serverVersion="dev">
            content
          </AppShell>
        }
      />,
    );

    expect(await screen.findByText("Loomarr dev")).not.toHaveAttribute("href");
    expect(screen.queryByRole("link", { name: /loomarr dev/i })).not.toBeInTheDocument();
  });

  it("gives an admin the full console", async () => {
    renderShell(true);
    expect(await screen.findByRole("link", { name: /settings/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /people/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /^requests$/i })).toBeInTheDocument();
  });

  // §12: TWO AUTHORED NAVS, not one list filtered by role. The member's is a different
  // product, not the admin's with gaps — so the admin-only surfaces are absent entirely
  // rather than present-and-greyed.
  // Home leads it too since #1659: the member Home carries no machine state.
  it("gives a member their own list, not the admin's list minus items", async () => {
    renderShell(false);
    expect(await screen.findByRole("link", { name: /^home$/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /guide/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /help/i })).toBeInTheDocument();
    for (const gone of [/settings/i, /people/i, /filler/i, /channels/i]) {
      expect(screen.queryByRole("link", { name: gone })).not.toBeInTheDocument();
    }
  });

  // The v2 mock's `navDefs`, with #1659's rename: Home · Guide · Requests · Filler · People ·
  // Settings · Help. Counted, not just spot-checked, because the count IS the claim — `Channels` and
  // `Suggest` folding into `/guide` is what took this from nine to seven, and a regression
  // would most likely show up as an extra entry rather than a wrong one.
  it("gives an admin exactly the mock's seven", async () => {
    renderShell(true);
    // Awaited: the RouterHarness resolves before Links render, so reading the DOM
    // synchronously finds an empty nav.
    const nav = await screen.findByRole("navigation", { name: "Primary" });
    const labels = within(nav)
      .getAllByRole("link")
      // The footer identity link (Your account) is not a nav section — it is you. Excluded
      // by its aria-label rather than its text, which is the avatar initials glued to the
      // name ("OPOperator") and would silently stop matching if either changed.
      .filter((a) => a.getAttribute("aria-label") !== "Your account")
      .map((a) => a.textContent?.trim());
    expect(labels).toEqual(["Home", "Guide", "Requests", "Filler", "People", "Settings", "Help"]);
  });

  // The palette opens on ⌘K or Ctrl K everywhere; the hint names the one this keyboard has.
  it("names the search shortcut for the viewer's platform", async () => {
    renderShell(true);
    const search = await screen.findByRole("button", { name: "Open global search" });
    const apple = /mac|iphone|ipad|ipod/i.test(navigator.platform);
    expect(search).toHaveTextContent(apple ? "⌘K" : "Ctrl K");
    expect(search).toHaveAttribute("aria-keyshortcuts", apple ? "Meta+K" : "Control+K");
  });

  // #1405: one name for everyone. Members used to see "My requests" and admins "Queue"; the page
  // is now the same Requests page for both, so the label no longer varies by role.
  it("names the shared route Requests for a member and an admin alike", async () => {
    renderShell(false);
    expect(await screen.findByRole("link", { name: "Requests" })).toHaveAttribute("href", "/requests");
  });

  it("hangs a count off the entry it is given, and nothing at zero", async () => {
    render(
      <RouterHarness
        content={
          <AppShell isAdmin={false} badges={{ "/requests": 3 }}>
            content
          </AppShell>
        }
      />,
    );
    expect(await screen.findByTestId("nav-badge-/requests")).toHaveTextContent("3");
  });

  it("shows no badge when the count is zero", async () => {
    render(
      <RouterHarness
        content={
          <AppShell isAdmin={false} badges={{ "/requests": 0 }}>
            content
          </AppShell>
        }
      />,
    );
    await screen.findByRole("link", { name: "Requests" });
    expect(screen.queryByTestId("nav-badge-/requests")).not.toBeInTheDocument();
  });

  // `Request a channel` was a member nav entry only while `/suggest` was its own page. It
  // folded into the Guide header (§12), where members get the affordance labelled for them —
  // so a second nav link to /guide would be a duplicate React key and two entries lit at
  // once, not an IA choice. Pinned because "restore the missing member entry" is a tempting
  // future change that would reintroduce exactly that.
  it("does not list a separate origination entry — it lives on the Guide", async () => {
    renderShell(false);
    expect(await screen.findByRole("link", { name: "Guide" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /request a channel/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /^suggest$/i })).not.toBeInTheDocument();
  });

  // PhoneBottomBar is `React.lazy` (it pulls in @loomarr/design-system's TabBar/BottomSheet,
  // which scripts/check-fe-bundle.mjs's initial-JS budget can't absorb as a static import — see
  // the component for why). The placeholder holds the bar's own 49px row open so the Guide's
  // docked strip above it doesn't jump once the chunk resolves and the real bar mounts.
  describe("below the tablet breakpoint", () => {
    it("holds the bar's row at a placeholder, then renders the bar itself", async () => {
      vi.stubGlobal("matchMedia", (query: string) => ({
        addEventListener: vi.fn(),
        matches: false,
        media: query,
        removeEventListener: vi.fn(),
      }));
      try {
        render(
          <LoomarrProvider theme="dark">
            <RouterHarness content={<AppShell isAdmin={false}>content</AppShell>} />
          </LoomarrProvider>,
        );

        expect(await screen.findByTestId("phone-bottom-bar-placeholder")).toHaveStyle({ height: "49px" });

        expect(await screen.findByRole("tablist", { name: "Primary navigation" })).toBeInTheDocument();
        expect(screen.queryByTestId("phone-bottom-bar-placeholder")).not.toBeInTheDocument();
      } finally {
        vi.unstubAllGlobals();
      }
    });
  });

  // #1817 item 3, decision X2: Watch is the rail's second entry — matching the position the
  // #1839 phone-bottom-bar draft already modelled for the same decision — in BOTH authored lists,
  // and absent (not greyed) with no channel. Checked for both roles because the rule has no
  // exception for either: Watch is a viewer destination first.
  describe("the Watch rail entry (decision X2)", () => {
    it("inserts Watch second for an admin once a channel exists", async () => {
      renderShellAt("/guide", { isAdmin: true, watchChannelId: "ch-7" });
      const nav = await screen.findByRole("navigation", { name: "Primary" });
      const labels = within(nav)
        .getAllByRole("link")
        .filter((a) => a.getAttribute("aria-label") !== "Your account")
        .map((a) => a.textContent?.trim());
      expect(labels).toEqual(["Home", "Watch", "Guide", "Requests", "Filler", "People", "Settings", "Help"]);
      expect(screen.getByRole("link", { name: "Watch" })).toHaveAttribute("href", "/channels/ch-7/watch");
    });

    it("inserts Watch second for a member once a channel exists", async () => {
      renderShellAt("/guide", { isAdmin: false, watchChannelId: "ch-7" });
      const nav = await screen.findByRole("navigation", { name: "Primary" });
      const labels = within(nav)
        .getAllByRole("link")
        .filter((a) => a.getAttribute("aria-label") !== "Your account")
        .map((a) => a.textContent?.trim());
      expect(labels).toEqual(["Home", "Watch", "Guide", "Requests", "Notifications", "Help"]);
    });

    it("hides Watch's slot entirely when no channel exists, for either role", async () => {
      renderShellAt("/guide", { isAdmin: true, watchChannelId: undefined });
      await screen.findByRole("link", { name: "Guide" });
      expect(screen.queryByRole("link", { name: "Watch" })).not.toBeInTheDocument();

      renderShellAt("/guide", { isAdmin: false, watchChannelId: undefined });
      expect(screen.queryAllByRole("link", { name: "Watch" })).toHaveLength(0);
    });
  });

  // Decision X2 (critique row 5): one channel entity never lights two rail items. Watch's own
  // route always highlights Watch; the channel's management tabs (Info/Programming/Filler/
  // Danger) highlight Guide instead — checked across all four sections and both roles.
  describe("nav highlight transfer on channel routes (decision X2)", () => {
    it("highlights Watch on any channel's watch route, not only the rail's own target", async () => {
      // A different channel ("$id") than the rail's own target ("ch-7") — any channel's Watch
      // route must light the rail's Watch entry, not just the one it links to.
      renderShellAt("/channels/$id/watch", { isAdmin: true, watchChannelId: "ch-7" });
      const watch = await screen.findByRole("link", { name: "Watch" });
      expect(watch).toHaveClass("bg-signal-tint-15", "text-signal");
      expect(screen.getByRole("link", { name: "Guide" })).not.toHaveClass("bg-signal-tint-15");
    });

    it.each(["info", "programming", "filler", "danger"] as const)(
      "highlights Guide, not Watch, on the channel's %s route (admin)",
      async (section) => {
        renderShellAt(`/channels/$id/${section}`, { isAdmin: true, watchChannelId: "ch-7" });
        const guide = await screen.findByRole("link", { name: "Guide" });
        expect(guide).toHaveClass("bg-signal-tint-15", "text-signal");
        expect(screen.getByRole("link", { name: "Watch" })).not.toHaveClass("bg-signal-tint-15");
      },
    );

    it("highlights Guide, not Watch, on the channel's info route (member)", async () => {
      renderShellAt("/channels/$id/info", { isAdmin: false, watchChannelId: "ch-7" });
      const guide = await screen.findByRole("link", { name: "Guide" });
      expect(guide).toHaveClass("bg-signal-tint-15", "text-signal");
      expect(screen.getByRole("link", { name: "Watch" })).not.toHaveClass("bg-signal-tint-15");
    });

    it("still highlights Guide normally when actually on /guide", async () => {
      renderShellAt("/guide", { isAdmin: true, watchChannelId: "ch-7" });
      expect(await screen.findByRole("link", { name: "Guide" })).toHaveAttribute("data-status", "active");
    });
  });
});
