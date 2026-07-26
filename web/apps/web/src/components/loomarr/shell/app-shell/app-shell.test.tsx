import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { AppShell } from "./app-shell";

const renderShell = (isAdmin: boolean) =>
  render(<RouterHarness content={<AppShell isAdmin={isAdmin}>content</AppShell>} />);

describe("AppShell", () => {
  it("gives an admin the full console", async () => {
    renderShell(true);
    expect(await screen.findByRole("link", { name: /settings/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /people/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /^queue$/i })).toBeInTheDocument();
  });

  // §12: TWO AUTHORED NAVS, not one list filtered by role. The member's is a different
  // product, not the admin's with gaps — so the admin-only surfaces are absent entirely
  // rather than present-and-greyed.
  it("gives a member their own four, not the admin's list minus items", async () => {
    renderShell(false);
    expect(await screen.findByRole("link", { name: /guide/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /help/i })).toBeInTheDocument();
    for (const gone of [/settings/i, /people/i, /filler/i, /channels/i]) {
      expect(screen.queryByRole("link", { name: gone })).not.toBeInTheDocument();
    }
  });

  // The SAME routes, named for what a member is doing with them. This is the thing a
  // role filter structurally could not do: it can hide an entry, never rename one.
  it("renames the shared routes for a member", async () => {
    renderShell(false);
    expect(await screen.findByRole("link", { name: "Request a channel" })).toHaveAttribute(
      "href",
      "/suggest",
    );
    expect(screen.getByRole("link", { name: "My requests" })).toHaveAttribute("href", "/queue");
    // The admin sees the operator-facing names for those same destinations.
    renderShell(true);
    expect(await screen.findByRole("link", { name: /^suggest$/i })).toBeInTheDocument();
  });
});
