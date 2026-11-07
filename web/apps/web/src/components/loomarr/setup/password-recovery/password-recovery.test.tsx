import { LoomarrProvider } from "@loomarr/design-system";
import { fireEvent, render as rtlRender, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { PasswordRecoveryRequest, PasswordRecoveryReset } from "./password-recovery";

// Field errors are design-system FieldHints, which need the theme the app mounts at its root. A
// wrapper rather than wrapping the element, so `rerender` keeps it.
const Themed = ({ children }: { children: ReactNode }) => (
  <LoomarrProvider theme="dark">{children}</LoomarrProvider>
);
const render = (ui: Parameters<typeof rtlRender>[0]) => rtlRender(ui, { wrapper: Themed });

describe("PasswordRecoveryRequest", () => {
  it("validates a username and always presents the generic sent state", async () => {
    const submit = vi.fn();
    const { rerender } = render(<PasswordRecoveryRequest onSubmit={submit} />);
    await userEvent.click(screen.getByRole("button", { name: /email reset link/i }));
    expect(screen.getByRole("alert")).toHaveTextContent("Enter your username");
    await userEvent.type(screen.getByLabelText("Username"), " Ada ");
    await userEvent.click(screen.getByRole("button", { name: /email reset link/i }));
    expect(submit).toHaveBeenCalledWith("Ada");
    rerender(<PasswordRecoveryRequest onSubmit={submit} sent />);
    expect(screen.getByRole("status")).toHaveTextContent(/if that account can be recovered here/i);
  });

  it("explains that imported passwords remain provider-owned", () => {
    render(<PasswordRecoveryRequest onSubmit={vi.fn()} />);
    expect(screen.getByText(/Emby or Jellyfin owns imported-account passwords/i)).toBeInTheDocument();
  });
});

describe("PasswordRecoveryReset", () => {
  it("requires matching eight-character passwords before explicit reset", async () => {
    const redeem = vi.fn();
    render(<PasswordRecoveryReset expiresAt={Date.now() + 60_000} onRedeem={redeem} />);
    fireEvent.change(screen.getByLabelText("New password"), { target: { value: "short" } });
    fireEvent.click(screen.getByRole("button", { name: "Reset password" }));
    expect(await screen.findByText("Use at least 8 characters.")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("New password"), { target: { value: "long-enough" } });
    fireEvent.change(screen.getByLabelText("Confirm new password"), { target: { value: "different" } });
    fireEvent.click(screen.getByRole("button", { name: "Reset password" }));
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("Passwords need to match"));
    expect(redeem).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("Confirm new password"), { target: { value: "long-enough" } });
    fireEvent.click(screen.getByRole("button", { name: "Reset password" }));
    await waitFor(() => expect(redeem).toHaveBeenCalledWith("long-enough"));
  });
});
