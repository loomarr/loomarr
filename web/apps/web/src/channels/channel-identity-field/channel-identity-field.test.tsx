import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ChannelIdentityField } from "./channel-identity-field";

// Purely presentational + a promise-returning onSave — no providers needed. The field owns
// the draft/dirty/save-state machine; these assert the confirmation contract: editing reveals
// Save/Cancel, Save commits the parsed value, Cancel/Escape reverts, an invalid draft blocks
// Save, and a rejected onSave (e.g. a 409 renumber) surfaces inline + keeps the editor open.

describe("ChannelIdentityField", () => {
  it("reveals Save/Cancel only when the value diverges from committed", async () => {
    const user = userEvent.setup();
    render(<ChannelIdentityField label="Channel name" value="90s Action" onSave={() => Promise.resolve()} />);

    // Idle: no confirmation controls.
    expect(screen.queryByRole("button", { name: /save channel name/i })).not.toBeInTheDocument();

    await user.type(screen.getByLabelText("Channel name"), " Redux");
    expect(screen.getByRole("button", { name: /save channel name/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /cancel channel name/i })).toBeInTheDocument();
  });

  it("Save calls onSave with the trimmed value", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(() => Promise.resolve());
    render(<ChannelIdentityField label="Channel name" value="90s Action" onSave={onSave} />);

    const input = screen.getByLabelText("Channel name");
    await user.clear(input);
    await user.type(input, "  Late Night  ");
    await user.click(screen.getByRole("button", { name: /save channel name/i }));

    expect(onSave).toHaveBeenCalledWith("Late Night");
  });

  it("Save on a number field passes a parsed number", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(() => Promise.resolve());
    render(<ChannelIdentityField label="Channel number" value={42} type="number" onSave={onSave} />);

    const input = screen.getByLabelText("Channel number");
    await user.clear(input);
    await user.type(input, "7");
    await user.click(screen.getByRole("button", { name: /save channel number/i }));

    expect(onSave).toHaveBeenCalledWith(7);
  });

  it("Cancel reverts the draft and hides the controls", async () => {
    const user = userEvent.setup();
    render(<ChannelIdentityField label="Channel name" value="90s Action" onSave={() => Promise.resolve()} />);

    const input = screen.getByLabelText("Channel name") as HTMLInputElement;
    await user.type(input, " Redux");
    await user.click(screen.getByRole("button", { name: /cancel channel name/i }));

    expect(input.value).toBe("90s Action");
    expect(screen.queryByRole("button", { name: /save channel name/i })).not.toBeInTheDocument();
  });

  it("Escape reverts, Enter saves", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(() => Promise.resolve());
    render(<ChannelIdentityField label="Channel name" value="A" onSave={onSave} />);
    const input = screen.getByLabelText("Channel name") as HTMLInputElement;

    await user.type(input, "B{Escape}");
    expect(input.value).toBe("A");

    await user.type(input, "C{Enter}");
    expect(onSave).toHaveBeenCalledWith("AC");
  });

  it("blocks Save on an invalid draft and shows the validation reason", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(() => Promise.resolve());
    render(
      <ChannelIdentityField
        label="Channel number"
        value={42}
        type="number"
        validate={(v) => (Number(v) < 1 ? "Use a whole number ≥ 1." : undefined)}
        onSave={onSave}
      />,
    );

    const input = screen.getByLabelText("Channel number");
    await user.clear(input);
    await user.type(input, "0");

    expect(screen.getByText("Use a whole number ≥ 1.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /save channel number/i })).toBeDisabled();
    await user.keyboard("{Enter}");
    expect(onSave).not.toHaveBeenCalled();
  });

  it("surfaces a rejected save inline and keeps the editor open (e.g. a 409 renumber)", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(() => Promise.reject(new Error("That channel number is already in use.")));
    render(<ChannelIdentityField label="Channel number" value={42} type="number" onSave={onSave} />);

    const input = screen.getByLabelText("Channel number");
    await user.clear(input);
    await user.type(input, "5");
    await user.click(screen.getByRole("button", { name: /save channel number/i }));

    // The reason shows inline, and the editor stays open (still dirty → Save still there).
    expect(await screen.findByText(/already in use/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /save channel number/i })).toBeInTheDocument();
  });

  it("adopts a new committed value from the server when the user hasn't diverged", async () => {
    const onSave = () => Promise.resolve();
    const { rerender } = render(
      <ChannelIdentityField label="Channel name" value="Old Name" onSave={onSave} />,
    );
    expect((screen.getByLabelText("Channel name") as HTMLInputElement).value).toBe("Old Name");

    // A background refresh (SSE) changes the committed value — the field adopts it.
    rerender(<ChannelIdentityField label="Channel name" value="New Name" onSave={onSave} />);
    await waitFor(() =>
      expect((screen.getByLabelText("Channel name") as HTMLInputElement).value).toBe("New Name"),
    );
  });
});
