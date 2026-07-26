import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SecretsPanel } from "./secrets-panel";
import type { SecretRow } from "./secrets-panel.type";

const SECRETS: SecretRow[] = [
  {
    name: "api_token",
    label: "API token",
    purpose: "Break-glass admin access for scripts and automation.",
    consequence: "The current token stops working immediately; scripts must be updated with the new one.",
    displayable: true,
  },
  {
    name: "session_secret",
    label: "Session secret",
    purpose: "Signs session cookies.",
    consequence: "Every session is revoked, including yours.",
    displayable: false,
  },
];

const render_ = (over = {}) =>
  render(
    <SecretsPanel secrets={SECRETS} revealed={{}} onReveal={vi.fn()} onRegenerate={vi.fn()} {...over} />,
  );

describe("SecretsPanel", () => {
  it("never offers to reveal a secret with nothing to paste (§4)", () => {
    render_();
    // One reveal button — the API token. SESSION_SECRET has no value affordance.
    expect(screen.getAllByRole("button", { name: /reveal/i })).toHaveLength(1);
  });

  it("states the consequence BEFORE regenerating, and needs a second confirm", async () => {
    const onRegenerate = vi.fn();
    render_({ onRegenerate });

    const [regen] = screen.getAllByRole("button", { name: /^regenerate$/i });
    await userEvent.click(regen as HTMLElement);

    // The click alone must not rotate anything — rotating breaks live clients.
    expect(onRegenerate).not.toHaveBeenCalled();
    expect(screen.getByText(/current token stops working/i)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /regenerate anyway/i }));
    expect(onRegenerate).toHaveBeenCalledWith("api_token");
  });

  it("lets the operator back out of a regeneration", async () => {
    const onRegenerate = vi.fn();
    render_({ onRegenerate });

    const [regen] = screen.getAllByRole("button", { name: /^regenerate$/i });
    await userEvent.click(regen as HTMLElement);
    await userEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(onRegenerate).not.toHaveBeenCalled();
    expect(screen.queryByText(/current token stops working/i)).not.toBeInTheDocument();
  });

  it("offers copy only once a value is on screen", async () => {
    const { rerender } = render_();
    expect(screen.queryByRole("button", { name: /copy/i })).not.toBeInTheDocument();

    rerender(
      <SecretsPanel
        secrets={SECRETS}
        revealed={{ api_token: "s3cr3t" }}
        onReveal={vi.fn()}
        onRegenerate={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: /copy/i })).toBeInTheDocument();
    expect(screen.getByText("s3cr3t")).toBeInTheDocument();
  });
});
