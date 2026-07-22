import type { ChannelPolicy } from "@loomarr/api";
import { render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { ChannelPolicyFields } from "./channel-policy-fields";

// Each field's help is a FieldHelp tooltip now, which needs a TooltipProvider ancestor (the
// app mounts one at the root). Wrap every render so the fields mount without a Radix error.
const render = (ui: ReactElement) => rtlRender(<TooltipProvider>{ui}</TooltipProvider>);

const EMPTY: ChannelPolicy = {};

// A populated policy carrying `applied` (reconcile-owned, never edited here) so tests
// can assert it survives every edit untouched.
const POPULATED: ChannelPolicy = {
  ordering: "shuffle",
  audience: { ceiling: "TV-14" },
  scope: { era: { from: 1990, to: 1999 } },
  separation: { movieNoRepeat: "168h", episodeNoRepeat: "24h" },
  applied: [{ kind: "blockMax", from: "8", to: "unbounded" }],
};

describe("ChannelPolicyFields", () => {
  it("renders inherit/no-limit sentinels for an empty policy", () => {
    render(<ChannelPolicyFields policy={EMPTY} onChange={vi.fn()} />);
    expect(screen.getByRole("combobox", { name: "Ordering" })).toHaveTextContent("Inherit channel default");
    expect(screen.getByRole("combobox", { name: "Audience ceiling" })).toHaveTextContent("No limit");
  });

  it("renders the current values of a populated policy", () => {
    render(<ChannelPolicyFields policy={POPULATED} onChange={vi.fn()} />);
    expect(screen.getByRole("combobox", { name: "Ordering" })).toHaveTextContent("Shuffled");
    expect(screen.getByRole("combobox", { name: "Audience ceiling" })).toHaveTextContent("TV-14");
    expect(screen.getByLabelText("From year")).toHaveValue(1990);
    expect(screen.getByLabelText("To year")).toHaveValue(1999);
    // Duration strings tidied for display (the wire form the operator reads/types).
    expect(screen.getByLabelText("Movies")).toHaveValue("168h");
    expect(screen.getByLabelText("Episodes")).toHaveValue("24h");
  });

  it("merges an ordering change into a NEW policy, preserving applied and other sections", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={POPULATED} onChange={onChange} />);

    await userEvent.click(screen.getByRole("combobox", { name: "Ordering" }));
    await userEvent.click(await screen.findByRole("option", { name: "In order" }));

    expect(onChange).toHaveBeenCalledWith({
      ...POPULATED,
      ordering: "sequential",
    });
    // applied survived, byref-equal — nothing rebuilt it.
    expect(onChange.mock.lastCall?.[0].applied).toBe(POPULATED.applied);
  });

  it("clears ordering back to inherit (empty string) via the sentinel", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={POPULATED} onChange={onChange} />);

    await userEvent.click(screen.getByRole("combobox", { name: "Ordering" }));
    await userEvent.click(await screen.findByRole("option", { name: "Inherit channel default" }));

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ ordering: "" }));
  });

  it("merges an audience ceiling change without disturbing scope/separation", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={POPULATED} onChange={onChange} />);

    await userEvent.click(screen.getByRole("combobox", { name: "Audience ceiling" }));
    await userEvent.click(await screen.findByRole("option", { name: "TV-MA" }));

    expect(onChange).toHaveBeenCalledWith({
      ...POPULATED,
      audience: { ceiling: "TV-MA" },
    });
  });

  it("commits an era edit on blur, not on keystroke", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={EMPTY} onChange={onChange} />);

    const from = screen.getByLabelText("From year");
    await userEvent.type(from, "1985");
    expect(onChange).not.toHaveBeenCalled();
    await userEvent.tab();

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ scope: { era: { from: 1985, to: undefined } } }),
    );
  });

  it("leaves era unchanged (no onChange) when blurring without editing", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={POPULATED} onChange={onChange} />);

    await userEvent.click(screen.getByLabelText("From year"));
    await userEvent.tab();

    expect(onChange).not.toHaveBeenCalled();
  });

  it("commits a no-repeat duration string on blur (the wire form)", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={EMPTY} onChange={onChange} />);

    const movies = screen.getByLabelText("Movies");
    await userEvent.type(movies, "168h");
    expect(onChange).not.toHaveBeenCalled();
    await userEvent.tab();

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ separation: { movieNoRepeat: "168h" } }));
  });

  it("clearing a no-repeat field commits undefined (no restriction), not zero", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={POPULATED} onChange={onChange} />);

    const movies = screen.getByLabelText("Movies");
    await userEvent.clear(movies);
    await userEvent.tab();

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        separation: { movieNoRepeat: undefined, episodeNoRepeat: POPULATED.separation?.episodeNoRepeat },
      }),
    );
  });
});
