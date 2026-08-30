import type { ImportCandidate } from "@loomarr/api/models/importCandidate";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { describe, expect, it, vi } from "vitest";
import { ImportDialog } from "./import-dialog";

const candidates: ImportCandidate[] = [
  { id: "ada", name: "Ada Lovelace", imported: false, disabled: false, isAdmin: true },
  { id: "grace", name: "Grace Hopper", imported: false, disabled: true, isAdmin: false },
  { id: "margaret", name: "Margaret Hamilton", imported: true, disabled: false, isAdmin: false },
];

const renderDialog = (props: Partial<ComponentProps<typeof ImportDialog>> = {}) => {
  const onImport = vi.fn().mockResolvedValue(undefined);
  const onSync = vi.fn().mockResolvedValue(undefined);
  render(
    <ImportDialog
      available
      candidates={candidates}
      defaultOpen
      onImport={onImport}
      onSync={onSync}
      {...props}
    />,
  );
  return { onImport, onSync };
};

describe("ImportDialog", () => {
  it("distinguishes imported, disabled, and server-admin accounts with initial roles", () => {
    renderDialog();
    const dialog = screen.getByRole("dialog", { name: "Import from Emby/Jellyfin" });
    expect(within(dialog).getByText("Already imported")).toBeInTheDocument();
    expect(within(dialog).getByText("Disabled on server")).toBeInTheDocument();
    expect(within(dialog).getByText("Server admin")).toBeInTheDocument();
    expect(within(dialog).getByText("Initial Loomarr role: Admin")).toBeInTheDocument();
    expect(within(dialog).getByLabelText(/^Margaret Hamilton/)).toBeDisabled();
  });

  it("selects only visible importable results and posts only explicit ids", async () => {
    const { onImport } = renderDialog();
    await userEvent.click(screen.getByLabelText(/^Ada Lovelace/));
    await userEvent.type(screen.getByLabelText("Search accounts"), "Grace");
    await userEvent.click(screen.getByRole("button", { name: "Select visible" }));
    expect(screen.getByText("2 selected · 1 importable shown")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Import 2 accounts" }));
    expect(onImport).toHaveBeenCalledWith(["ada", "grace"]);
    await waitFor(() => expect(screen.getByText("0 selected · 1 importable shown")).toBeInTheDocument());
  });

  it("clears only visible selected results", async () => {
    renderDialog();
    await userEvent.click(screen.getByLabelText(/^Ada Lovelace/));
    await userEvent.click(screen.getByLabelText(/^Grace Hopper/));
    await userEvent.type(screen.getByLabelText("Search accounts"), "Grace");
    await userEvent.click(screen.getByRole("button", { name: "Clear visible" }));
    expect(screen.getByText("1 selected · 1 importable shown")).toBeInTheDocument();
  });

  it("filters the loaded candidate set without selecting hidden rows", async () => {
    const user = userEvent.setup();
    renderDialog();
    await user.click(screen.getByRole("combobox", { name: "Show" }));
    await user.click(await screen.findByRole("option", { name: "Already imported" }));
    expect(screen.getByText("Margaret Hamilton")).toBeInTheDocument();
    expect(screen.queryByText("Ada Lovelace")).not.toBeInTheDocument();
    expect(screen.getByText("0 selected · 0 importable shown")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Select visible" })).toBeDisabled();
  });

  it("retains selection when import fails", async () => {
    const onImport = vi.fn().mockRejectedValue(new Error("provider unavailable"));
    renderDialog({ onImport });
    await userEvent.click(screen.getByLabelText(/^Ada Lovelace/));
    await userEvent.click(screen.getByRole("button", { name: "Import 1 account" }));
    await waitFor(() => expect(onImport).toHaveBeenCalledOnce());
    expect(screen.getByRole("button", { name: "Import 1 account" })).toBeEnabled();
  });

  it("explains the disconnected state and links to Connections", () => {
    renderDialog({ available: false, candidates: undefined });
    expect(screen.getByText("Connect a media server first")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open Connections settings" })).toHaveAttribute(
      "href",
      "/settings/connections",
    );
    expect(screen.queryByRole("button", { name: "Sync existing" })).not.toBeInTheDocument();
  });

  it("keeps sync explicit and describes credential refresh", async () => {
    const { onSync } = renderDialog();
    expect(screen.getByText(/non-reversible Argon2id verifier/i)).toBeInTheDocument();
    expect(screen.getByText(/refreshes it after later provider sign-ins/i)).toBeInTheDocument();
    expect(screen.getByText(/never provisions an unselected account/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Sync existing" }));
    expect(onSync).toHaveBeenCalledOnce();
  });

  it("closes with Escape and returns focus to the header action", async () => {
    renderDialog({ defaultOpen: false });
    const trigger = screen.getByRole("button", { name: "Import from Emby/Jellyfin" });
    await userEvent.click(trigger);
    expect(screen.getByRole("dialog", { name: "Import from Emby/Jellyfin" })).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Import from Emby/Jellyfin" })).not.toBeInTheDocument(),
    );
    expect(trigger).toHaveFocus();
  });
});
