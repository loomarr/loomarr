import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PasswordRecoveryRequest, PasswordRecoveryReset } from "./password-recovery";

describe("PasswordRecoveryRequest", () => {
  it("validates a username and always presents the generic sent state", async () => {
    const submit = vi.fn();
    const { rerender } = render(<PasswordRecoveryRequest onSubmit={submit} />);
    await userEvent.click(screen.getByRole("button", { name: /email reset link/i }));
    expect(screen.getByRole("alert")).toHaveTextContent("Enter your username");
    await userEvent.type(screen.getByLabelText("Username"), "Ada");
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
  it("requires matching eight-character passwords before explicit reset", () => {
    const redeem = vi.fn();
    render(<PasswordRecoveryReset expiresAt={Date.now() + 60_000} onRedeem={redeem} />);
    fireEvent.change(screen.getByLabelText("New password"), { target: { value: "long-enough" } });
    fireEvent.change(screen.getByLabelText("Confirm new password"), { target: { value: "different" } });
    fireEvent.click(screen.getByRole("button", { name: "Reset password" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Passwords need to match");
    fireEvent.change(screen.getByLabelText("Confirm new password"), { target: { value: "long-enough" } });
    fireEvent.click(screen.getByRole("button", { name: "Reset password" }));
    expect(redeem).toHaveBeenCalledWith("long-enough");
  });
});
