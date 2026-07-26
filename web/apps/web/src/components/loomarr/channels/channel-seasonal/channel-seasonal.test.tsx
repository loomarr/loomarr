import type { ChannelPolicy, Vocabulary } from "@loomarr/api";
import { render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { ChannelSeasonal } from "./channel-seasonal";

const render = (ui: ReactElement) => rtlRender(<TooltipProvider>{ui}</TooltipProvider>);

// A vocabulary shaped like the BE's: holiday tokens live in `when` as `holiday:<id>`, mixed in
// with the non-holiday WHEN tokens the picker must ignore.
const VOCAB = {
  when: [
    { token: "primetime", label: "Primetime (20–23)", shortLabel: "Primetime", priority: 1, predicate: {} },
    { token: "holiday:christmas", label: "Christmas", shortLabel: "Christmas", priority: 2, predicate: {} },
    { token: "holiday:halloween", label: "Halloween", shortLabel: "Halloween", priority: 2, predicate: {} },
    { token: "holiday:newyear", label: "New Year", shortLabel: "New Year", priority: 2, predicate: {} },
  ],
  what: [],
  how: [],
} as unknown as Vocabulary;

const POPULATED: ChannelPolicy = {
  ordering: "shuffle",
  applied: [{ kind: "blockMax", from: "8", to: "unbounded" }],
};

describe("ChannelSeasonal", () => {
  it("renders the default sentinel for an unset seasonal policy", () => {
    render(<ChannelSeasonal policy={POPULATED} onChange={vi.fn()} vocabulary={VOCAB} />);
    expect(screen.getByRole("combobox", { name: "Holidays" })).toHaveTextContent("Automatic");
  });

  // The holiday list comes from the BE vocabulary, never a hand-mirrored copy: BuildVocabulary
  // lowers each token through LowerWhen → knownHoliday → builtinCalendar, so a holiday the
  // engine does not know cannot be offered, and one it gains appears with no FE change.
  it("lists holidays from the vocabulary, ignoring non-holiday WHEN tokens", () => {
    render(<ChannelSeasonal policy={POPULATED} onChange={vi.fn()} vocabulary={VOCAB} />);

    expect(screen.getByLabelText("Christmas")).toBeInTheDocument();
    expect(screen.getByLabelText("Halloween")).toBeInTheDocument();
    expect(screen.queryByLabelText("Primetime (20–23)")).not.toBeInTheDocument();
  });

  it("commits a mode change, preserving the rest of the policy", async () => {
    const onChange = vi.fn();
    render(<ChannelSeasonal policy={POPULATED} onChange={onChange} vocabulary={VOCAB} />);

    await userEvent.click(screen.getByRole("combobox", { name: "Holidays" }));
    await userEvent.click(await screen.findByRole("option", { name: "Holiday channel" }));

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ seasonal: { mode: "exclusive" } }));
    expect(onChange.mock.lastCall?.[0].applied).toBe(POPULATED.applied);
  });

  it("clears mode back to the default sentinel (empty string)", async () => {
    const onChange = vi.fn();
    render(
      <ChannelSeasonal policy={{ seasonal: { mode: "exclusive" } }} onChange={onChange} vocabulary={VOCAB} />,
    );

    await userEvent.click(screen.getByRole("combobox", { name: "Holidays" }));
    await userEvent.click(await screen.findByRole("option", { name: "Automatic" }));

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ seasonal: { mode: "" } }));
  });

  it("ticking a holiday adds its id; unticking removes it", async () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <ChannelSeasonal policy={POPULATED} onChange={onChange} vocabulary={VOCAB} />,
    );

    await userEvent.click(screen.getByLabelText("Halloween"));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ seasonal: { holidays: ["halloween"] } }));

    rerender(
      <TooltipProvider>
        <ChannelSeasonal
          policy={{ ...POPULATED, seasonal: { holidays: ["halloween"] } }}
          onChange={onChange}
          vocabulary={VOCAB}
        />
      </TooltipProvider>,
    );
    await userEvent.click(screen.getByLabelText("Halloween"));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ seasonal: { holidays: [] } }));
  });

  // Stored in vocabulary order regardless of tick order, so re-saving an unchanged selection
  // does not reshuffle the array and show up as a spurious diff.
  it("stores holidays in vocabulary order, not click order", async () => {
    const onChange = vi.fn();
    render(
      <ChannelSeasonal
        policy={{ seasonal: { holidays: ["newyear"] } }}
        onChange={onChange}
        vocabulary={VOCAB}
      />,
    );

    await userEvent.click(screen.getByLabelText("Christmas"));

    // christmas precedes newyear in the vocabulary, so it leads even though it was ticked second.
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ seasonal: { holidays: ["christmas", "newyear"] } }),
    );
  });

  // An empty selection means ALL built-in holidays (schedule/seasonal.go:78 — "empty subset =
  // all built-in holidays"), which is the opposite of what an empty list usually implies. The
  // legend says so rather than leaving the operator to guess that none means every.
  it("says an empty selection means all holidays", () => {
    render(<ChannelSeasonal policy={POPULATED} onChange={vi.fn()} vocabulary={VOCAB} />);
    expect(screen.getByText(/none picked — all of them/)).toBeInTheDocument();
  });

  it("hides the holiday list when holidays are ignored", () => {
    render(<ChannelSeasonal policy={{ seasonal: { mode: "off" } }} onChange={vi.fn()} vocabulary={VOCAB} />);
    expect(screen.queryByLabelText("Christmas")).not.toBeInTheDocument();
  });

  // offSeason is read ONLY inside `case SeasonalExclusive` (schedule/seasonal.go:154), so
  // offering it under any other mode would be a control the engine never consults.
  it("shows the off-season fallback only in exclusive mode", () => {
    const { unmount } = render(
      <ChannelSeasonal policy={{ seasonal: { mode: "auto" } }} onChange={vi.fn()} vocabulary={VOCAB} />,
    );
    expect(screen.queryByRole("combobox", { name: "Out of season" })).not.toBeInTheDocument();
    unmount();

    render(
      <ChannelSeasonal policy={{ seasonal: { mode: "exclusive" } }} onChange={vi.fn()} vocabulary={VOCAB} />,
    );
    expect(screen.getByRole("combobox", { name: "Out of season" })).toBeInTheDocument();
  });

  it("commits an off-season choice in exclusive mode", async () => {
    const onChange = vi.fn();
    render(
      <ChannelSeasonal policy={{ seasonal: { mode: "exclusive" } }} onChange={onChange} vocabulary={VOCAB} />,
    );

    await userEvent.click(screen.getByRole("combobox", { name: "Out of season" }));
    await userEvent.click(await screen.findByRole("option", { name: "Go dark" }));

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ seasonal: { mode: "exclusive", offSeason: "dark" } }),
    );
  });

  // The mode names are not self-explanatory ("auto" vs "exclusive" decides what airs in
  // December), so each carries a sentence. Pinned so the copy cannot drift from the mode.
  it("explains what the selected mode does", () => {
    const { unmount } = render(
      <ChannelSeasonal policy={{ seasonal: { mode: "exclusive" } }} onChange={vi.fn()} vocabulary={VOCAB} />,
    );
    expect(screen.getByText(/the channel IS the holiday/i)).toBeInTheDocument();
    unmount();

    render(<ChannelSeasonal policy={{ seasonal: { mode: "off" } }} onChange={vi.fn()} vocabulary={VOCAB} />);
    expect(screen.getByText(/nothing is boosted or benched/i)).toBeInTheDocument();
  });
});
