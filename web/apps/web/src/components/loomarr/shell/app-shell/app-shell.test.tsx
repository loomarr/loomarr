import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { AppShell } from "./app-shell";

const renderShell = (isAdmin: boolean) =>
  render(<RouterHarness content={<AppShell isAdmin={isAdmin}>content</AppShell>} />);

describe("AppShell", () => {
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
  it("gives a member their own three, not the admin's list minus items", async () => {
    renderShell(false);
    expect(await screen.findByRole("link", { name: /guide/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /help/i })).toBeInTheDocument();
    for (const gone of [/settings/i, /people/i, /filler/i, /channels/i, /dashboard/i]) {
      expect(screen.queryByRole("link", { name: gone })).not.toBeInTheDocument();
    }
  });

  // The v2 mock's `navDefs` verbatim: Dashboard · Guide · Requests · Filler · People · Settings
  // · Help. Counted, not just spot-checked, because the count IS the claim — `Channels` and
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
    expect(labels).toEqual(["Dashboard", "Guide", "Requests", "Filler", "People", "Settings", "Help"]);
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
});
