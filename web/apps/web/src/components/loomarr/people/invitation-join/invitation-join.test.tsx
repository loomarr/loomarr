import type { InvitationPreviewOutputBody } from "@loomarr/api/models/invitationPreviewOutputBody";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { InvitationJoin } from "./invitation-join";

const localPreview: InvitationPreviewOutputBody = {
  kind: "local",
  username: "Ada",
  role: "admin",
  credentialPath: "local_password",
  expiresAt: Date.UTC(2030, 2, 24, 12),
};

describe("InvitationJoin", () => {
  it("explains the reserved identity and waits for explicit matching-password consent", () => {
    const redeem = vi.fn();
    render(<InvitationJoin preview={localPreview} onRedeem={redeem} />);

    expect(screen.getByRole("heading", { name: "Join Loomarr" })).toBeInTheDocument();
    expect(screen.getByText("Ada")).toBeInTheDocument();
    expect(screen.getByText(/administrator access/i)).toBeInTheDocument();
    expect(redeem).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Create password"), { target: { value: "long-enough" } });
    fireEvent.change(screen.getByLabelText("Confirm password"), { target: { value: "different" } });
    fireEvent.click(screen.getByRole("button", { name: "Activate account" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Passwords need to match");
    expect(redeem).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Confirm password"), { target: { value: "long-enough" } });
    fireEvent.click(screen.getByRole("button", { name: "Activate account" }));
    expect(redeem).toHaveBeenCalledWith({ password: "long-enough" });
  });

  it("asks imported people for their current Library identity and password", () => {
    const redeem = vi.fn();
    render(
      <InvitationJoin
        preview={{
          ...localPreview,
          kind: "library",
          username: undefined,
          displayName: "Grace Hopper",
          role: "member",
          credentialPath: "library_password",
        }}
        onRedeem={redeem}
      />,
    );

    expect(screen.getByText("Grace Hopper")).toBeInTheDocument();
    expect(screen.getByLabelText("Library username")).toBeInTheDocument();
    expect(screen.getByLabelText("Library password")).toHaveAttribute("autocomplete", "current-password");
    expect(screen.queryByLabelText("Confirm password")).not.toBeInTheDocument();
  });
});
