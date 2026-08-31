import { people } from "@loomarr/fixtures";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PersonDetail } from "./person-detail";

const renderDetail = (overrides = {}) =>
  render(<PersonDetail user={people.localAdmin} sessions={[]} {...overrides} />);

describe("PersonDetail", () => {
  it("persists independent role, quota, and approval changes", async () => {
    const onRoleChange = vi.fn();
    const onQuotaChange = vi.fn();
    const onToggleAutoApprove = vi.fn();
    renderDetail({ onRoleChange, onQuotaChange, onToggleAutoApprove });

    await userEvent.click(screen.getByLabelText("Role"));
    await userEvent.click(await screen.findByRole("option", { name: "Member" }));
    expect(onRoleChange).toHaveBeenCalledWith("member");
    const quota = screen.getByLabelText("Quota");
    await userEvent.clear(quota);
    await userEvent.type(quota, "12");
    expect(onQuotaChange).not.toHaveBeenCalled();
    await userEvent.tab();
    expect(onQuotaChange).toHaveBeenCalledWith(12);
    await userEvent.click(screen.getByLabelText("Auto-approve requests"));
    expect(onToggleAutoApprove).toHaveBeenCalledWith(false);
  });

  it("refuses self-demotion and self-disable", () => {
    renderDetail({ isSelf: true });
    expect(screen.getByLabelText("Role")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Disable account" })).toBeDisabled();
  });

  it("resets only local passwords and validates their length", async () => {
    const onResetPassword = vi.fn().mockResolvedValue(undefined);
    renderDetail({ onResetPassword });
    await userEvent.click(screen.getByRole("button", { name: "Reset password" }));
    await userEvent.type(screen.getByLabelText("New password"), "short");
    await userEvent.click(screen.getByRole("button", { name: "Set new password" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Use at least 8 characters.");
    await userEvent.clear(screen.getByLabelText("New password"));
    await userEvent.type(screen.getByLabelText("New password"), "long-enough");
    await userEvent.click(screen.getByRole("button", { name: "Set new password" }));
    expect(onResetPassword).toHaveBeenCalledWith("long-enough");
  });

  it("explains provider-owned password changes and Loomarr's offline verifier", () => {
    render(<PersonDetail user={people.importedMember} sessions={[]} onResetPassword={vi.fn()} />);
    expect(screen.getByText(/change this password in the connected media server/i)).toBeInTheDocument();
    expect(screen.getByText(/argon2id verifier for offline login/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reset password" })).not.toBeInTheDocument();
  });

  it("adds an unverified contact without presenting it as a login identifier", async () => {
    const onSetContactAddress = vi.fn().mockResolvedValue(undefined);
    renderDetail({ onSetContactAddress });
    expect(screen.getByText(/email never replaces this person's sign-in name/i)).toBeInTheDocument();
    expect(screen.getByText(/qr and direct account access still work/i)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Add email" }));
    await userEvent.type(screen.getByLabelText("Contact email"), "invalid");
    await userEvent.click(screen.getByRole("button", { name: "Save email" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Enter one complete email address.");
    expect(onSetContactAddress).not.toHaveBeenCalled();

    await userEvent.clear(screen.getByLabelText("Contact email"));
    await userEvent.type(screen.getByLabelText("Contact email"), "ada@example.com");
    await userEvent.click(screen.getByRole("button", { name: "Save email" }));
    expect(onSetContactAddress).toHaveBeenCalledWith("ada@example.com");
  });

  it("keeps a verified address recovery-capable while its replacement is pending", async () => {
    const onCancelContactReplacement = vi.fn().mockResolvedValue(undefined);
    const onRemoveContactAddress = vi.fn().mockResolvedValue(undefined);
    render(
      <PersonDetail
        user={{
          ...people.localAdmin,
          contactAddress: {
            email: "ada@example.com",
            status: "verified",
            provenance: "invitation",
            verifiedAt: 1_900_000_000_000,
          },
          contactReplacement: {
            email: "ada.new@example.com",
            status: "pending",
            provenance: "admin",
          },
        }}
        sessions={[]}
        onCancelContactReplacement={onCancelContactReplacement}
        onRemoveContactAddress={onRemoveContactAddress}
      />,
    );
    expect(screen.getByText("Verified")).toBeInTheDocument();
    expect(screen.getByText("Pending replacement")).toBeInTheDocument();
    expect(screen.getByText(/verified address above remains recovery-capable/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Cancel replacement" }));
    expect(onCancelContactReplacement).toHaveBeenCalledOnce();
    await userEvent.click(screen.getByRole("button", { name: "Remove email" }));
    expect(onRemoveContactAddress).toHaveBeenCalledOnce();
  });
});
