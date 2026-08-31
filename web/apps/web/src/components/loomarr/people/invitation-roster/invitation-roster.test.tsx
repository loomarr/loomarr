import type { InvitationBody } from "@loomarr/api/models/invitationBody";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { InvitationRoster } from "./invitation-roster";

const invitation = (overrides: Partial<InvitationBody> = {}): InvitationBody => ({
  id: "invitation-1",
  kind: "local",
  username: "Ada",
  role: "member",
  status: "pending",
  createdAt: Date.UTC(2030, 0, 1),
  expiresAt: Date.UTC(2030, 0, 8),
  contactAddress: { email: "ada@example.com", status: "pending", provenance: "admin" },
  ...overrides,
});

describe("InvitationRoster", () => {
  it("composes invitation lifecycle with actionable email delivery state", async () => {
    const onSendEmail = vi.fn();
    render(
      <InvitationRoster
        invitations={[
          invitation({
            emailDelivery: {
              status: "failed",
              outcome: "recipient_rejected",
              attemptNumber: 1,
              updatedAt: Date.UTC(2030, 0, 2),
            },
          }),
          invitation({ id: "invitation-2", username: "Grace", status: "redeemed", redeemedBy: "user-2" }),
        ]}
        sendingId={undefined}
        onSendEmail={onSendEmail}
      />,
    );

    expect(screen.getByText("Email failed")).toBeInTheDocument();
    expect(screen.getByText(/rejected that recipient/i)).toBeInTheDocument();
    expect(screen.getByText("Redeemed")).toBeInTheDocument();
    expect(screen.getAllByText(/copy or qr/i).length).toBeGreaterThan(0);

    await userEvent.click(screen.getByRole("button", { name: "Retry email to Ada" }));
    expect(onSendEmail).toHaveBeenCalledWith("invitation-1");
  });

  it("does not offer email without a contact address or for a terminal invitation", () => {
    render(
      <InvitationRoster
        invitations={[
          invitation({ contactAddress: undefined }),
          invitation({ id: "invitation-2", username: "Grace", status: "revoked" }),
        ]}
        onSendEmail={() => {}}
      />,
    );

    expect(screen.getByText("Add a contact address before sending email.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /email to Grace/i })).not.toBeInTheDocument();
  });
});
