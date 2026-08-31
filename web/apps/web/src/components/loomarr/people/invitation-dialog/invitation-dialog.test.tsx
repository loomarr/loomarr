import type { CreateInvitationInputBody } from "@loomarr/api/models/createInvitationInputBody";
import type { InvitationBody } from "@loomarr/api/models/invitationBody";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { InvitationDialog } from "./invitation-dialog";

const expiresAt = Date.UTC(2030, 0, 8, 12);
const localInvitation: InvitationBody = {
  id: "invitation-local",
  kind: "local",
  username: "newcomer",
  role: "member",
  status: "pending",
  createdAt: Date.UTC(2030, 0, 1, 12),
  expiresAt,
};

const renderDialog = (overrides: Partial<React.ComponentProps<typeof InvitationDialog>> = {}) => {
  const onCreate = vi
    .fn<(input: CreateInvitationInputBody) => Promise<InvitationBody>>()
    .mockResolvedValue(localInvitation);
  const onIssueGrant = vi.fn().mockResolvedValue({
    url: "https://loomarr.example/join#grant=fake-deterministic-grant",
    expiresAt,
  });
  const onRevoke = vi.fn().mockResolvedValue(undefined);
  const onSendEmail = vi.fn().mockResolvedValue(undefined);
  render(
    <InvitationDialog
      defaultOpen
      libraryAvailable
      candidates={[
        { id: "ada", name: "Ada", imported: false, disabled: false, isAdmin: true },
        { id: "disabled", name: "Disabled", imported: false, disabled: true, isAdmin: false },
      ]}
      onCreate={onCreate}
      onIssueGrant={onIssueGrant}
      onRevoke={onRevoke}
      onSendEmail={onSendEmail}
      {...overrides}
    />,
  );
  return { onCreate, onIssueGrant, onRevoke, onSendEmail };
};

describe("InvitationDialog", () => {
  beforeEach(() => {
    Object.defineProperty(globalThis.navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  it("creates a member by default and presents one in-memory link as QR and copy", async () => {
    const { onCreate, onIssueGrant } = renderDialog();

    expect(screen.getByRole("combobox", { name: "Initial role" })).toHaveTextContent("Member");
    await userEvent.type(screen.getByLabelText("Reserved username"), "  newcomer  ");
    await userEvent.type(screen.getByLabelText("Contact email (optional)"), "person@example.com");
    await userEvent.click(screen.getByRole("button", { name: "Create invitation" }));

    await waitFor(() => expect(onIssueGrant).toHaveBeenCalledWith("invitation-local", "qr"));
    expect(onCreate).toHaveBeenCalledWith({
      kind: "local",
      username: "newcomer",
      contactEmail: "person@example.com",
      role: "member",
    });
    expect(screen.getByRole("img", { name: "Scan to accept newcomer’s Loomarr invitation" })).toBeVisible();
    expect(screen.getByText(/Expires January 8, 2030/)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Copy invitation link" }));
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
      "https://loomarr.example/join#grant=fake-deterministic-grant",
    );
    expect(screen.getByRole("button", { name: "Copied" })).toBeInTheDocument();
  });

  it("requires an explicit Library account and explicit administrator choice", async () => {
    const { onCreate } = renderDialog();
    await userEvent.click(screen.getByRole("radio", { name: "Emby or Jellyfin account" }));
    await userEvent.click(screen.getByRole("combobox", { name: "Library account" }));
    await userEvent.click(await screen.findByRole("option", { name: /Ada/ }));
    expect(screen.getByText("Media-server role: Administrator")).toBeInTheDocument();
    expect(screen.getByText("Initial Loomarr role: Member")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("combobox", { name: "Initial role" }));
    await userEvent.click(await screen.findByRole("option", { name: "Administrator" }));
    expect(screen.getByText("Initial Loomarr role: Administrator")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Create invitation" }));

    await waitFor(() =>
      expect(onCreate).toHaveBeenCalledWith({
        kind: "library",
        libraryUserId: "ada",
        contactEmail: "",
        role: "admin",
      }),
    );
  });

  it("can request email without making QR and copy depend on delivery", async () => {
    const failingEmail = vi.fn().mockRejectedValue(new Error("smtp unavailable"));
    const { onIssueGrant } = renderDialog({ onSendEmail: failingEmail });
    await userEvent.type(screen.getByLabelText("Reserved username"), "newcomer");
    await userEvent.type(screen.getByLabelText("Contact email (optional)"), "person@example.com");
    await userEvent.click(screen.getByRole("checkbox", { name: /Also send by email/ }));
    await userEvent.click(screen.getByRole("button", { name: "Create invitation" }));

    await waitFor(() => expect(failingEmail).toHaveBeenCalledWith("invitation-local"));
    expect(onIssueGrant).toHaveBeenCalledWith("invitation-local", "qr");
    expect(screen.getByRole("img", { name: /Scan to accept/ })).toBeVisible();
    expect(screen.getByRole("alert")).toHaveTextContent(/email could not be queued/i);
  });

  it("regenerates and revokes an existing invitation, clearing the prior bearer", async () => {
    const replacement = "https://loomarr.example/join#grant=fake-replacement-grant";
    const onIssueGrant = vi
      .fn()
      .mockResolvedValueOnce({ url: "https://loomarr.example/join#grant=fake-first-grant", expiresAt })
      .mockResolvedValueOnce({ url: replacement, expiresAt });
    const { onRevoke } = renderDialog({ existing: localInvitation, onIssueGrant });

    await userEvent.click(screen.getByRole("button", { name: "Generate QR and link" }));
    expect(await screen.findByRole("button", { name: "Regenerate link" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Regenerate link" }));
    await userEvent.click(screen.getByRole("button", { name: "Copy invitation link" }));
    expect(navigator.clipboard.writeText).toHaveBeenLastCalledWith(replacement);

    await userEvent.click(screen.getByRole("button", { name: "Revoke invitation" }));
    await waitFor(() => expect(onRevoke).toHaveBeenCalledWith("invitation-local"));
    expect(screen.getByText("Invitation revoked")).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: /Scan to accept/ })).not.toBeInTheDocument();
  });

  it("keeps the QR available and reports a clipboard rejection without exposing the bearer", async () => {
    vi.mocked(navigator.clipboard.writeText).mockRejectedValueOnce(new Error("clipboard denied"));
    renderDialog({ existing: localInvitation });

    await userEvent.click(screen.getByRole("button", { name: "Generate QR and link" }));
    await userEvent.click(await screen.findByRole("button", { name: "Copy invitation link" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/could not be copied/i);
    expect(screen.getByRole("img", { name: /Scan to accept/ })).toBeVisible();
    expect(screen.queryByText(/fake-deterministic-grant/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy invitation link" })).toBeInTheDocument();
  });

  it("clears bearer and editable values whenever the dialog closes", async () => {
    renderDialog();
    await userEvent.type(screen.getByLabelText("Reserved username"), "newcomer");
    await userEvent.click(screen.getByRole("button", { name: "Create invitation" }));
    expect(await screen.findByRole("img", { name: /Scan to accept/ })).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");

    await userEvent.click(screen.getByRole("button", { name: "Invite person" }));
    expect(screen.getByLabelText("Reserved username")).toHaveValue("");
    expect(screen.queryByText(/fake-deterministic-grant/)).not.toBeInTheDocument();
    expect(screen.queryByRole("img", { name: /Scan to accept/ })).not.toBeInTheDocument();
  });
});
