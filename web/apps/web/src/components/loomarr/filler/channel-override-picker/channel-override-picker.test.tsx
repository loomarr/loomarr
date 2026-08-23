import { channelFits } from "@loomarr/fixtures";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ChannelOverridePicker } from "./channel-override-picker";

const noop = () => {};
const base = {
  clipName: "Frosted Flakes — They're Grrreat!",
  channels: channelFits,
  onSet: noop,
  onReset: noop,
};

const row = (name: string) => screen.getByText(name).closest("li") as HTMLElement;

describe("ChannelOverridePicker", () => {
  it("explains that pinning overrides automatic rotation", () => {
    render(<ChannelOverridePicker {...base} />);

    expect(screen.getByText(/pin takes priority over automatic rotation/i)).toHaveTextContent(
      /may repeat inside the cooldown or reduce variety/i,
    );
  });

  // ⚠ THE load-bearing one. The domain has three states and a checkbox has two; an automatic
  // channel must NOT read as blocked, or an operator sees their catalog apparently switched off
  // everywhere and starts ticking boxes to "fix" it.
  it("shows an untouched channel as automatic, not blocked", () => {
    render(<ChannelOverridePicker {...base} />);

    const r = row("Saturday Mornings");
    expect(within(r).getByText("automatic")).toBeInTheDocument();
    expect(within(r).queryByText("blocked")).not.toBeInTheDocument();
    // And there is no way back FROM automatic, because it is already there.
    expect(within(r).queryByRole("button", { name: /Back to automatic/ })).not.toBeInTheDocument();
  });

  // ⚠ A pin has NO rung — it is placed ahead of the ladder — so the server sends
  // `bumper_card` with no reason. Read naively that is "won't be picked", which is the exact
  // opposite of what a pin means.
  it("reads a pinned channel as always played, never as unpickable", () => {
    render(<ChannelOverridePicker {...base} />);

    const r = row("Retro Movies");
    expect(within(r).getByText("always played here")).toBeInTheDocument();
    expect(within(r).getByText("always")).toBeInTheDocument();
    expect(within(r).queryByText(/won't be picked/)).not.toBeInTheDocument();
  });

  // The reason this endpoint returns a code at all: "won't be picked" with no cause sends an
  // operator hunting through channel settings for a rule they cannot see.
  it("says WHY an automatic channel would never pick the clip", () => {
    render(<ChannelOverridePicker {...base} />);

    expect(within(row("Kids Block")).getByText("wrong audience for this channel")).toBeInTheDocument();
  });

  it("names the ladder rung for a channel that would pick it", () => {
    render(<ChannelOverridePicker {...base} />);

    expect(within(row("Saturday Mornings")).getByText("matches this channel exactly")).toBeInTheDocument();
    expect(within(row("Late Night Sci-Fi")).getByText("close enough — same decade")).toBeInTheDocument();
  });

  // ⚠ Ticking sends BOTH flags, because "not pinned" and "excluded" are different states. A
  // single boolean would make unticking a channel block it — silently, and everywhere.
  it("sends an explicit pinned/excluded pair, not one flag", async () => {
    const onSet = vi.fn();
    render(<ChannelOverridePicker {...base} onSet={onSet} />);

    await userEvent.click(within(row("Saturday Mornings")).getByRole("checkbox", { name: /Always play/ }));

    expect(onSet).toHaveBeenCalledWith("ch-1", { pinned: true, excluded: false });
  });

  // ⚠ **This test used to assert the OPPOSITE, and that is why the bug read as deliberate
  // (§10 V51f).** It was named "unticking an overridden channel blocks it rather than clearing
  // the override" and pinned `{pinned: false, excluded: true}` — so releasing a pin silently moved
  // the clip to "never play here", on a component whose own header ⚠ warns that collapsing the
  // third state "silently blocks an operator's catalog". A green test stating the trap as the
  // contract is worse than no test: it makes the behaviour look chosen, and anyone who noticed it
  // would assume they were the one who was wrong.
  it("unticking returns a channel to automatic instead of blocking it", async () => {
    const onSet = vi.fn();
    const onReset = vi.fn();
    render(<ChannelOverridePicker {...base} onSet={onSet} onReset={onReset} />);

    await userEvent.click(within(row("Retro Movies")).getByRole("checkbox", { name: /Always play/ }));

    expect(onReset).toHaveBeenCalledWith("ch-4");
    expect(onSet).not.toHaveBeenCalled();
  });

  // Blocking is a decision, so it gets a button of its own rather than riding on the checkbox an
  // operator uses to un-pin.
  it("blocks only through the explicit Block action", async () => {
    const onSet = vi.fn();
    render(<ChannelOverridePicker {...base} onSet={onSet} />);

    await userEvent.click(within(row("Saturday Mornings")).getByRole("button", { name: /Block/ }));

    expect(onSet).toHaveBeenCalledWith("ch-1", { pinned: false, excluded: true });
  });

  // ⚠ ...and Block is offered only where there is nothing to undo. An overridden row already has
  // "Automatic", and stacking a third control on it invites the same "which one releases this?"
  // confusion the checkbox caused.
  it("offers Block only on an automatic channel", () => {
    render(<ChannelOverridePicker {...base} />);

    expect(within(row("Saturday Mornings")).queryByRole("button", { name: /Block/ })).toBeInTheDocument();
    expect(within(row("Newsreel")).queryByRole("button", { name: /Block/ })).not.toBeInTheDocument();
  });

  // ⚠ The only route back to the third state. A checkbox cannot express "automatic", so without
  // this an operator who ticks a channel once can never undo it.
  it("offers a way back to automatic, and only on an overridden channel", async () => {
    const onReset = vi.fn();
    render(<ChannelOverridePicker {...base} onReset={onReset} />);

    await userEvent.click(within(row("Newsreel")).getByRole("button", { name: /Back to automatic/ }));

    expect(onReset).toHaveBeenCalledWith("ch-5");
  });

  it("renders a blocked channel as blocked", () => {
    render(<ChannelOverridePicker {...base} />);

    const r = row("Newsreel");
    expect(within(r).getByText("blocked")).toBeInTheDocument();
    expect(within(r).getByText("you've blocked it here")).toBeInTheDocument();
  });

  // The mode note carries the meaning the checkboxes cannot: that unticked is a decision only
  // once the row is overridden.
  //
  // ⚠ **This asserted only that the sentence EXISTED (`/picks channels for .* automatically/`),
  // which is why it kept passing while the sentence was wrong.** The copy said "untick to block
  // it" for the whole life of the untick bug, and went on saying it after the behaviour changed —
  // through every green run of this file. Found by looking at a regenerated visual baseline, not
  // by a test. It now asserts what the note actually INSTRUCTS, so copy and behaviour cannot
  // drift apart silently again.
  it("explains what the controls do, in terms that match what they do", () => {
    render(<ChannelOverridePicker {...base} />);

    const note = screen.getByText(/picks channels for .* automatically/i);
    expect(note).toHaveTextContent("Untick to hand the choice back to Loomarr");
    expect(note).toHaveTextContent("use Block to keep it off that channel");
    expect(note).not.toHaveTextContent(/untick to block/i);
  });

  // One row's write must not disable the whole list — the mutation's own isPending is global.
  it("disables only the channel being written", () => {
    render(<ChannelOverridePicker {...base} busyChannelId="ch-1" />);

    expect(within(row("Saturday Mornings")).getByRole("checkbox", { name: /Always play/ })).toBeDisabled();
    expect(within(row("Late Night Sci-Fi")).getByRole("checkbox", { name: /Always play/ })).toBeEnabled();
  });

  // ⚠ Every checkbox would otherwise be named "checkbox", so a screen-reader user hears an
  // undifferentiated list and cannot tell which channel they are on. Third occurrence of this
  // class in the repo, so it is asserted rather than assumed.
  it("names the channel each checkbox acts on", () => {
    render(<ChannelOverridePicker {...base} />);

    const names = screen.getAllByRole("checkbox").map((c) => c.getAttribute("aria-label"));
    expect(new Set(names).size).toBe(names.length);
    for (const n of names) expect(n).toMatch(/Saturday|Late Night|Kids|Retro|Newsreel/);
  });

  it("says so when there are no channels at all", () => {
    render(<ChannelOverridePicker {...base} channels={[]} />);

    expect(screen.getByText(/nowhere to play/i)).toBeInTheDocument();
  });
});
