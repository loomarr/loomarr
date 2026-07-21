import type { UserBody } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { UserRow } from "./user-row";

const user = (over: Partial<UserBody> = {}): UserBody => ({
  id: "u1",
  name: "Ada",
  role: "member",
  quota: 5,
  disabled: false,
  autoApprove: false,
  local: false,
  pendingAcquisitions: 2,
  effectiveQuota: 5,
  ...over,
});

describe("UserRow", () => {
  it("states the credential path, because it decides what actions apply", () => {
    const { rerender } = render(<UserRow user={user({ local: true })} />);
    expect(screen.getByText(/local account/i)).toBeInTheDocument();
    rerender(<UserRow user={user({ local: false })} />);
    expect(screen.getByText(/media-server account/i)).toBeInTheDocument();
  });

  it("writes each field independently, with no save step", () => {
    const onRoleChange = vi.fn();
    const onToggleAutoApprove = vi.fn();
    render(<UserRow user={user()} onRoleChange={onRoleChange} onToggleAutoApprove={onToggleAutoApprove} />);

    return (async () => {
      await userEvent.click(screen.getByLabelText("Role"));
      await userEvent.click(await screen.findByRole("option", { name: "admin" }));
      expect(onRoleChange).toHaveBeenCalledWith("admin");
      await userEvent.click(screen.getByLabelText(/auto-approve/i));
      expect(onToggleAutoApprove).toHaveBeenCalledWith(true);
    })();
  });

  // Typing "12" must not PATCH a quota of 1 on the way there.
  it("commits quota on blur, not per keystroke", async () => {
    const onQuotaChange = vi.fn();
    render(<UserRow user={user({ quota: 5 })} onQuotaChange={onQuotaChange} />);

    const field = screen.getByLabelText("Quota");
    await userEvent.clear(field);
    await userEvent.type(field, "12");
    expect(onQuotaChange).not.toHaveBeenCalled();

    await userEvent.tab();
    expect(onQuotaChange).toHaveBeenCalledExactlyOnceWith(12);
  });

  it("does not re-send an unchanged quota on blur", async () => {
    const onQuotaChange = vi.fn();
    render(<UserRow user={user({ quota: 5 })} onQuotaChange={onQuotaChange} />);
    await userEvent.click(screen.getByLabelText("Quota"));
    await userEvent.tab();
    expect(onQuotaChange).not.toHaveBeenCalled();
  });

  // The one destructive action with no in-app undo: an admin who demotes or disables
  // themselves cannot reverse it, and if they are the last admin nobody can.
  it("refuses self-demotion and self-disable", () => {
    render(<UserRow user={user({ role: "admin" })} isSelf />);
    expect(screen.getByLabelText("Role")).toBeDisabled();
    expect(screen.getByRole("button", { name: /disable/i })).toBeDisabled();
    expect(screen.getByText("You")).toBeInTheDocument();
  });

  it("offers Enable for a disabled user", async () => {
    const onToggleDisabled = vi.fn();
    render(<UserRow user={user({ disabled: true })} onToggleDisabled={onToggleDisabled} />);
    expect(screen.getByText("Disabled")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /enable/i }));
    expect(onToggleDisabled).toHaveBeenCalledWith(false);
  });

  it("disables its controls while a change to this row is in flight", () => {
    render(<UserRow user={user()} busy onViewSessions={() => {}} />);
    expect(screen.getByLabelText("Role")).toBeDisabled();
    expect(screen.getByLabelText("Quota")).toBeDisabled();
    expect(screen.getByRole("button", { name: /sessions/i })).toBeDisabled();
  });

  // The denominator was deliberately withheld until quota actually bound something
  // (§11 auto-approve): showing "2 / 5" while nothing enforced the 5 would advertise a
  // limit that did not exist.
  it("shows pending acquisitions against the effective cap", () => {
    render(<UserRow user={user({ pendingAcquisitions: 3, quota: 0, effectiveQuota: 5 })} />);
    expect(screen.getByText("3 /")).toBeInTheDocument();
    // A quota of 0 means "use the default", so the field shows the server's effective
    // value as a placeholder rather than a literal 0 the admin would read as "none".
    expect(screen.getByLabelText("Quota")).toHaveAttribute("placeholder", "5");
  });
});
