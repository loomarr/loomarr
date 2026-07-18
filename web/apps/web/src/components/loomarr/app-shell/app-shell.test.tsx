import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { AppShell } from "./app-shell";

const renderShell = (isAdmin: boolean) =>
  render(
    <MemoryRouter>
      <AppShell isAdmin={isAdmin}>content</AppShell>
    </MemoryRouter>,
  );

describe("AppShell", () => {
  it("shows admin-only nav to admins", () => {
    renderShell(true);
    expect(screen.getByRole("link", { name: /settings/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /users/i })).toBeInTheDocument();
  });

  it("hides admin-only nav from members", () => {
    renderShell(false);
    expect(screen.queryByRole("link", { name: /settings/i })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: /channels/i })).toBeInTheDocument();
  });
});
