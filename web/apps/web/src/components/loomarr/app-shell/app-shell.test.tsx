import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { AppShell } from "./app-shell";

const renderShell = (isAdmin: boolean) =>
  render(<RouterHarness content={<AppShell isAdmin={isAdmin}>content</AppShell>} />);

describe("AppShell", () => {
  it("shows admin-only nav to admins", async () => {
    renderShell(true);
    expect(await screen.findByRole("link", { name: /settings/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /users/i })).toBeInTheDocument();
  });

  it("hides admin-only nav from members", async () => {
    renderShell(false);
    expect(await screen.findByRole("link", { name: /channels/i })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /settings/i })).not.toBeInTheDocument();
  });
});
