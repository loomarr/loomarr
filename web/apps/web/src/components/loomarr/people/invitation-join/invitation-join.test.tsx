import type { InvitationPreviewOutputBody } from "@loomarr/api/models/invitationPreviewOutputBody";
import { LoomarrProvider } from "@loomarr/design-system";
import { fireEvent, render as rtlRender, screen, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { InvitationJoin } from "./invitation-join";

// Field errors are design-system FieldHints, which need the theme the app mounts at its root.
const render = (ui: ReactElement) => rtlRender(<LoomarrProvider theme="dark">{ui}</LoomarrProvider>);

const localPreview: InvitationPreviewOutputBody = {
  kind: "local",
  username: "Adela",
  role: "admin",
  credentialPath: "local_password",
  expiresAt: Date.UTC(2030, 2, 24, 12),
};

describe("InvitationJoin", () => {
  it("explains the reserved identity and waits for explicit matching-password consent", async () => {
    const redeem = vi.fn();
    render(<InvitationJoin preview={localPreview} onRedeem={redeem} />);

    expect(screen.getByRole("heading", { name: "Join Loomarr" })).toBeInTheDocument();
    expect(screen.getByText("Adela")).toBeInTheDocument();
    expect(screen.getByText(/administrator access/i)).toBeInTheDocument();
    expect(redeem).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Create password"), { target: { value: "long-enough" } });
    fireEvent.change(screen.getByLabelText("Confirm password"), { target: { value: "different" } });
    fireEvent.click(screen.getByRole("button", { name: "Activate account" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Passwords need to match");
    expect(screen.getByLabelText("Confirm password")).toHaveAttribute("aria-invalid", "true");
    expect(redeem).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Confirm password"), { target: { value: "long-enough" } });
    fireEvent.click(screen.getByRole("button", { name: "Activate account" }));
    await waitFor(() => expect(redeem).toHaveBeenCalledWith({ password: "long-enough" }));
  });

  it("asks imported people for their current Library identity and password", async () => {
    const redeem = vi.fn();
    render(
      <InvitationJoin
        preview={{
          ...localPreview,
          kind: "library",
          username: undefined,
          displayName: "Greta Finch",
          role: "member",
          credentialPath: "library_password",
        }}
        onRedeem={redeem}
      />,
    );

    expect(screen.getByText("Greta Finch")).toBeInTheDocument();
    expect(screen.getByLabelText("Library username")).toBeInTheDocument();
    expect(screen.getByLabelText("Library password")).toHaveAttribute("autocomplete", "current-password");
    expect(screen.queryByLabelText("Confirm password")).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Library password"), { target: { value: "long-enough" } });
    fireEvent.click(screen.getByRole("button", { name: "Activate account" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Enter the username for the invited Library account.",
    );
    expect(redeem).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Library username"), { target: { value: "  greta  " } });
    fireEvent.click(screen.getByRole("button", { name: "Activate account" }));
    await waitFor(() => expect(redeem).toHaveBeenCalledWith({ username: "greta", password: "long-enough" }));
  });
});
