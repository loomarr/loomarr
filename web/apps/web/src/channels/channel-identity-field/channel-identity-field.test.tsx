import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { ChannelIdentityField } from "./channel-identity-field";

// Presentational + a promise-returning onSave — only the theme provider. The field owns
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
    expect(await screen.findByRole("alert")).toHaveTextContent(/already in use/i);
    expect(screen.getByRole("button", { name: /save channel number/i })).toBeInTheDocument();
  });

  it("closes the editor once the saved, trimmed value comes back as committed", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(() => Promise.resolve());
    const { rerender } = render(
      <ChannelIdentityField label="Channel name" value="90s Action" onSave={onSave} />,
    );

    const input = screen.getByLabelText("Channel name") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "  Late Night  ");
    await user.click(screen.getByRole("button", { name: /save channel name/i }));
    expect(onSave).toHaveBeenCalledWith("Late Night");
    // Until the parent passes the stored value, the input keeps showing what was saved.
    await waitFor(() => expect(input).toBeEnabled());
    expect(input.value).toBe("Late Night");

    // The parent re-renders with the value the server stored: the normalised draft.
    rerender(<ChannelIdentityField label="Channel name" value="Late Night" onSave={onSave} />);
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /save channel name/i })).not.toBeInTheDocument(),
    );
    expect(input.value).toBe("Late Night");
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

  it("adopts a server change that arrives after a save has come back", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(() => Promise.resolve());
    const { rerender } = render(<ChannelIdentityField label="Channel name" value="A" onSave={onSave} />);
    const input = screen.getByLabelText("Channel name") as HTMLInputElement;

    await user.type(input, "B{Enter}");
    expect(onSave).toHaveBeenCalledWith("AB");
    rerender(<ChannelIdentityField label="Channel name" value="AB" onSave={onSave} />);

    // Someone else renames the channel afterwards: the field shows their name.
    rerender(<ChannelIdentityField label="Channel name" value="Elsewhere" onSave={onSave} />);
    await waitFor(() => expect(input.value).toBe("Elsewhere"));
  });

  it("adopts a server change once the draft is typed back to the stored value", async () => {
    const user = userEvent.setup();
    const onSave = () => Promise.resolve();
    const { rerender } = render(<ChannelIdentityField label="Channel name" value="A" onSave={onSave} />);
    const input = screen.getByLabelText("Channel name") as HTMLInputElement;

    await user.type(input, "B{Backspace}");
    expect(input.value).toBe("A");

    rerender(<ChannelIdentityField label="Channel name" value="Elsewhere" onSave={onSave} />);
    await waitFor(() => expect(input.value).toBe("Elsewhere"));
  });

  it("keeps an edit in progress when the server value changes underneath it", async () => {
    const user = userEvent.setup();
    const onSave = () => Promise.resolve();
    const { rerender } = render(<ChannelIdentityField label="Channel name" value="A" onSave={onSave} />);
    const input = screen.getByLabelText("Channel name") as HTMLInputElement;

    await user.type(input, "B");
    rerender(<ChannelIdentityField label="Channel name" value="Elsewhere" onSave={onSave} />);

    expect(input.value).toBe("AB");
    expect(screen.getByRole("button", { name: /save channel name/i })).toBeInTheDocument();
  });

  it("adopts later server changes once the server arrives at the draft", async () => {
    const user = userEvent.setup();
    const onSave = () => Promise.resolve();
    const { rerender } = render(<ChannelIdentityField label="Channel name" value="A" onSave={onSave} />);
    const input = screen.getByLabelText("Channel name") as HTMLInputElement;

    await user.type(input, "B");
    // Another tab saved the same name: there is nothing left to save.
    rerender(<ChannelIdentityField label="Channel name" value="AB" onSave={onSave} />);
    expect(screen.queryByRole("button", { name: /save channel name/i })).not.toBeInTheDocument();

    rerender(<ChannelIdentityField label="Channel name" value="Elsewhere" onSave={onSave} />);
    await waitFor(() => expect(input.value).toBe("Elsewhere"));
  });

  it("keeps the saved value when another change arrives before the save comes back", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(() => Promise.resolve());
    const { rerender } = render(<ChannelIdentityField label="Channel name" value="A" onSave={onSave} />);
    const input = screen.getByLabelText("Channel name") as HTMLInputElement;

    await user.type(input, "B{Enter}");
    await waitFor(() => expect(input).toBeEnabled());
    rerender(<ChannelIdentityField label="Channel name" value="Elsewhere" onSave={onSave} />);

    expect(input.value).toBe("AB");
    expect(screen.getByRole("button", { name: /save channel name/i })).toBeInTheDocument();
  });

  it("keeps the draft through a server change that lands while a save is in flight", async () => {
    const user = userEvent.setup();
    let finish = () => {};
    const onSave = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    const { rerender } = render(<ChannelIdentityField label="Channel name" value="A" onSave={onSave} />);
    const input = screen.getByLabelText("Channel name") as HTMLInputElement;

    await user.type(input, "B{Enter}");
    expect(input).toBeDisabled();
    rerender(<ChannelIdentityField label="Channel name" value="Elsewhere" onSave={onSave} />);
    expect(input.value).toBe("AB");
    expect(input).toBeDisabled();

    finish();
    await waitFor(() => expect(input).toBeEnabled());
    expect(input.value).toBe("AB");
  });

  it("stays disabled until the save resolves, even when the stored value arrives first", async () => {
    const user = userEvent.setup();
    let finish = () => {};
    const onSave = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    const { rerender } = render(<ChannelIdentityField label="Channel name" value="A" onSave={onSave} />);
    const input = screen.getByLabelText("Channel name") as HTMLInputElement;

    await user.type(input, "B{Enter}");
    // The mutation's refetch lands before onSave resolves.
    rerender(<ChannelIdentityField label="Channel name" value="AB" onSave={onSave} />);
    expect(input).toBeDisabled();

    finish();
    await waitFor(() => expect(input).toBeEnabled());
    expect(input.value).toBe("AB");
    expect(screen.queryByRole("button", { name: /save channel name/i })).not.toBeInTheDocument();
  });

  it("after a rejected save, keeps the draft and the reason through a server change, and Cancel shows the latest stored value", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(() => Promise.reject(new Error("That name is taken.")));
    const { rerender } = render(<ChannelIdentityField label="Channel name" value="A" onSave={onSave} />);
    const input = screen.getByLabelText("Channel name") as HTMLInputElement;

    await user.type(input, "B{Enter}");
    expect(await screen.findByRole("alert")).toHaveTextContent("That name is taken.");
    rerender(<ChannelIdentityField label="Channel name" value="Elsewhere" onSave={onSave} />);

    expect(input.value).toBe("AB");
    expect(screen.getByRole("alert")).toHaveTextContent("That name is taken.");

    await user.click(screen.getByRole("button", { name: /cancel channel name/i }));
    expect(input.value).toBe("Elsewhere");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("clears a rejected save's reason once the draft is edited", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(() => Promise.reject(new Error("That name is taken.")));
    render(<ChannelIdentityField label="Channel name" value="A" onSave={onSave} />);
    const input = screen.getByLabelText("Channel name");

    await user.type(input, "B{Enter}");
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    await user.type(input, "C");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  // Below `sm` the channel header gives the name its own row, and the field wraps there instead of
  // cutting a long name (#1785). It is still one line of content.
  describe("below sm", () => {
    beforeEach(() => {
      vi.stubGlobal("matchMedia", (query: string) => ({
        matches: false,
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }));
      return () => vi.unstubAllGlobals();
    });

    it("renders the title as a wrapping textarea, and the number stays an input", () => {
      render(
        <>
          <ChannelIdentityField label="Channel name" value="Saturday Morning Cartoons" onSave={vi.fn()} />
          <ChannelIdentityField
            label="Channel number"
            value={7}
            type="number"
            variant="compact"
            onSave={vi.fn()}
          />
        </>,
      );
      expect(screen.getByLabelText("Channel name").tagName).toBe("TEXTAREA");
      expect(screen.getByLabelText("Channel number").tagName).toBe("INPUT");
    });

    it("saves on Enter without breaking the line", async () => {
      const user = userEvent.setup();
      const onSave = vi.fn(() => Promise.resolve());
      render(<ChannelIdentityField label="Channel name" value="Kettle Kids" onSave={onSave} />);

      await user.type(screen.getByLabelText("Channel name"), " Weekend{Enter}");
      expect(onSave).toHaveBeenCalledWith("Kettle Kids Weekend");
      expect(screen.getByLabelText("Channel name")).not.toHaveValue(expect.stringContaining("\n"));
    });

    it("turns a pasted line break into a space", async () => {
      const user = userEvent.setup();
      render(<ChannelIdentityField label="Channel name" value="" onSave={vi.fn()} />);

      await user.click(screen.getByLabelText("Channel name"));
      await user.paste("Harbour Towns\nin the\r\nOff Season");
      expect(screen.getByLabelText("Channel name")).toHaveValue("Harbour Towns in the Off Season");
    });
  });
});
