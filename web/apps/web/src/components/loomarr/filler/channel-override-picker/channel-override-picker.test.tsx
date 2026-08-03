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

  it("unticking an overridden channel blocks it rather than clearing the override", async () => {
    const onSet = vi.fn();
    render(<ChannelOverridePicker {...base} onSet={onSet} />);

    await userEvent.click(within(row("Retro Movies")).getByRole("checkbox", { name: /Always play/ }));

    expect(onSet).toHaveBeenCalledWith("ch-4", { pinned: false, excluded: true });
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
  it("explains what the checkboxes mean before you use them", () => {
    render(<ChannelOverridePicker {...base} />);

    expect(screen.getByText(/picks channels for .* automatically/i)).toBeInTheDocument();
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
