import type { ChannelPolicy, Vocabulary } from "@loomarr/api";
import { LoomarrProvider } from "@loomarr/design-system";
import { render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { ChannelSeasonal } from "./channel-seasonal";

// LoomarrProvider as main.tsx mounts it: the seasonal fields are design-system views.
const render = (ui: ReactElement) => rtlRender(<LoomarrProvider theme="dark">{ui}</LoomarrProvider>);

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

const radio = (name: string) => screen.getByRole("radio", { name });
const holiday = (name: string) => screen.getByRole("checkbox", { name });

describe("ChannelSeasonal", () => {
  it("names the built-in source for an unset seasonal policy", () => {
    render(<ChannelSeasonal policy={POPULATED} onChange={vi.fn()} vocabulary={VOCAB} />);
    expect(radio("Use built-in seasonal rotation")).toBeChecked();
  });

  it("collapses the explicit auto wire value into the same built-in choice", () => {
    render(<ChannelSeasonal policy={{ seasonal: { mode: "auto" } }} onChange={vi.fn()} vocabulary={VOCAB} />);
    expect(radio("Use built-in seasonal rotation")).toBeChecked();
  });

  // The holiday list comes from the BE vocabulary, never a hand-mirrored copy: BuildVocabulary
  // lowers each token through LowerWhen → knownHoliday → builtinCalendar, so a holiday the
  // engine does not know cannot be offered, and one it gains appears with no FE change.
  it("lists holidays from the vocabulary, ignoring non-holiday WHEN tokens", () => {
    render(<ChannelSeasonal policy={POPULATED} onChange={vi.fn()} vocabulary={VOCAB} />);

    expect(holiday("Christmas")).toBeInTheDocument();
    expect(holiday("Halloween")).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "Primetime (20–23)" })).not.toBeInTheDocument();
  });

  it("commits a mode change, preserving the rest of the policy", async () => {
    const onChange = vi.fn();
    render(<ChannelSeasonal policy={POPULATED} onChange={onChange} vocabulary={VOCAB} />);

    await userEvent.click(radio("Holiday-only channel"));

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ seasonal: { mode: "exclusive" } }));
    expect(onChange.mock.lastCall?.[0].applied).toBe(POPULATED.applied);
  });

  it("clears mode back to the default sentinel (empty string)", async () => {
    const onChange = vi.fn();
    render(
      <ChannelSeasonal policy={{ seasonal: { mode: "exclusive" } }} onChange={onChange} vocabulary={VOCAB} />,
    );

    await userEvent.click(radio("Use built-in seasonal rotation"));

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ seasonal: { mode: "" } }));
  });

  it("ticking a holiday adds its id; unticking removes it", async () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <ChannelSeasonal
        policy={{ ...POPULATED, seasonal: { holidays: ["christmas"] } }}
        onChange={onChange}
        vocabulary={VOCAB}
      />,
    );

    await userEvent.click(holiday("Halloween"));
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ seasonal: { holidays: ["christmas", "halloween"] } }),
    );

    rerender(
      <LoomarrProvider theme="dark">
        <ChannelSeasonal
          policy={{ ...POPULATED, seasonal: { holidays: ["christmas", "halloween"] } }}
          onChange={onChange}
          vocabulary={VOCAB}
        />
      </LoomarrProvider>,
    );
    await userEvent.click(holiday("Halloween"));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ seasonal: { holidays: ["christmas"] } }));
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

    await userEvent.click(holiday("Christmas"));

    // christmas precedes newyear in the vocabulary, so it leads even though it was ticked second.
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ seasonal: { holidays: ["christmas", "newyear"] } }),
    );
  });

  // An empty WIRE selection means ALL built-in holidays (schedule/seasonal.go:78). Render the
  // effective state, not the sentinel, so the checkbox contract stays conventional.
  it("shows every holiday checked when the empty wire subset means all", () => {
    render(<ChannelSeasonal policy={POPULATED} onChange={vi.fn()} vocabulary={VOCAB} />);
    expect(screen.getByText("All built-in holidays are included.")).toBeInTheDocument();
    expect(holiday("Christmas")).toBeChecked();
    expect(holiday("Halloween")).toBeChecked();
    expect(holiday("New Year")).toBeChecked();
  });

  it("materializes the remaining subset when one built-in holiday is unchecked", async () => {
    const onChange = vi.fn();
    render(<ChannelSeasonal policy={POPULATED} onChange={onChange} vocabulary={VOCAB} />);

    await userEvent.click(holiday("Halloween"));

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ seasonal: { holidays: ["christmas", "newyear"] } }),
    );
  });

  it("hides the holiday list when holidays are ignored", () => {
    render(<ChannelSeasonal policy={{ seasonal: { mode: "off" } }} onChange={vi.fn()} vocabulary={VOCAB} />);
    expect(screen.queryByRole("checkbox", { name: "Christmas" })).not.toBeInTheDocument();
  });

  // offSeason is read ONLY inside `case SeasonalExclusive` (schedule/seasonal.go:154), so
  // offering it under any other mode would be a control the engine never consults.
  it("shows the off-season fallback only in exclusive mode", () => {
    const { unmount } = render(
      <ChannelSeasonal policy={{ seasonal: { mode: "auto" } }} onChange={vi.fn()} vocabulary={VOCAB} />,
    );
    expect(screen.queryByRole("radiogroup", { name: "Out of season" })).not.toBeInTheDocument();
    unmount();

    render(
      <ChannelSeasonal policy={{ seasonal: { mode: "exclusive" } }} onChange={vi.fn()} vocabulary={VOCAB} />,
    );
    expect(screen.getByRole("radiogroup", { name: "Out of season" })).toBeInTheDocument();
  });

  it("commits an off-season choice in exclusive mode", async () => {
    const onChange = vi.fn();
    render(
      <ChannelSeasonal policy={{ seasonal: { mode: "exclusive" } }} onChange={onChange} vocabulary={VOCAB} />,
    );

    await userEvent.click(radio("Go dark"));

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ seasonal: { mode: "exclusive", offSeason: "dark" } }),
    );
  });

  it("collapses the explicit loop wire value into regular programming", () => {
    render(
      <ChannelSeasonal
        policy={{ seasonal: { mode: "exclusive", offSeason: "loop" } }}
        onChange={vi.fn()}
        vocabulary={VOCAB}
      />,
    );
    expect(radio("Keep regular programming")).toBeChecked();
  });

  // The mode names are not self-explanatory ("auto" vs "exclusive" decides what airs in
  // December), so each carries a sentence. Pinned so the copy cannot drift from the mode.
  it("explains what each mode does", () => {
    render(
      <ChannelSeasonal policy={{ seasonal: { mode: "exclusive" } }} onChange={vi.fn()} vocabulary={VOCAB} />,
    );
    expect(screen.getByText(/the channel IS the holiday/i)).toBeInTheDocument();
    expect(screen.getByText(/nothing is boosted or benched/i)).toBeInTheDocument();
  });

  // The mode is the defaultable field (#1817 item 4); "auto" is the default spelled out.
  it("badges the built-in mode Default with Reset disabled", () => {
    render(<ChannelSeasonal policy={{ seasonal: { mode: "auto" } }} onChange={vi.fn()} vocabulary={VOCAB} />);
    expect(screen.getByText("Default")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reset to default (Seasonal behavior)" })).toBeDisabled();
  });

  it("resets an overridden mode to the empty sentinel, keeping the holidays", async () => {
    const onChange = vi.fn();
    const policy: ChannelPolicy = { ...POPULATED, seasonal: { mode: "exclusive", holidays: ["christmas"] } };
    render(<ChannelSeasonal policy={policy} onChange={onChange} vocabulary={VOCAB} />);
    expect(screen.getByText("Channel override")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Reset to default (Seasonal behavior)" }));
    expect(onChange).toHaveBeenCalledWith({ ...POPULATED, seasonal: { mode: "", holidays: ["christmas"] } });
  });

  // Holiday strength + Favour evenings shape only the built-in rotation (§6 "Holiday episodes").
  describe("holiday episodes", () => {
    it("offers strength and evenings only with the built-in rotation", () => {
      const { unmount } = render(<ChannelSeasonal policy={{}} onChange={vi.fn()} vocabulary={VOCAB} />);
      expect(radio("Featured")).toBeChecked();
      expect(screen.getByRole("checkbox", { name: "Favour evenings" })).toBeChecked();
      unmount();

      for (const mode of ["exclusive", "off"]) {
        const view = render(
          <ChannelSeasonal policy={{ seasonal: { mode } }} onChange={vi.fn()} vocabulary={VOCAB} />,
        );
        expect(screen.queryByRole("radiogroup", { name: "Holiday strength" })).not.toBeInTheDocument();
        expect(screen.queryByRole("checkbox", { name: "Favour evenings" })).not.toBeInTheDocument();
        view.unmount();
      }
    });

    it("writes the chosen strength, and Featured as the default sentinel", async () => {
      const onChange = vi.fn();
      const { rerender } = render(
        <ChannelSeasonal policy={POPULATED} onChange={onChange} vocabulary={VOCAB} />,
      );
      await userEvent.click(radio("Heavy"));
      expect(onChange).toHaveBeenLastCalledWith({ ...POPULATED, seasonal: { strength: "heavy" } });

      rerender(
        <LoomarrProvider theme="dark">
          <ChannelSeasonal
            policy={{ ...POPULATED, seasonal: { strength: "heavy" } }}
            onChange={onChange}
            vocabulary={VOCAB}
          />
        </LoomarrProvider>,
      );
      expect(radio("Heavy")).toBeChecked();
      await userEvent.click(radio("Featured"));
      expect(onChange).toHaveBeenLastCalledWith({ ...POPULATED, seasonal: { strength: "" } });
    });

    // Placement is `"" = evenings`: switching evenings off must write the explicit opt-out, and
    // switching it back on must write the sentinel, never a value that reads back differently.
    it("writes the evenings opt-out explicitly and reads it back", async () => {
      const onChange = vi.fn();
      const { rerender } = render(<ChannelSeasonal policy={{}} onChange={onChange} vocabulary={VOCAB} />);
      expect(screen.getByText(/between 5 and 11 pm/i)).toBeInTheDocument();
      await userEvent.click(screen.getByRole("checkbox", { name: "Favour evenings" }));
      expect(onChange).toHaveBeenLastCalledWith({ seasonal: { placement: "anytime" } });

      rerender(
        <LoomarrProvider theme="dark">
          <ChannelSeasonal
            policy={{ seasonal: { placement: "anytime" } }}
            onChange={onChange}
            vocabulary={VOCAB}
          />
        </LoomarrProvider>,
      );
      expect(screen.getByRole("checkbox", { name: "Favour evenings" })).not.toBeChecked();
      expect(screen.getByText(/spread across the whole day/i)).toBeInTheDocument();
      await userEvent.click(screen.getByRole("checkbox", { name: "Favour evenings" }));
      expect(onChange).toHaveBeenLastCalledWith({ seasonal: { placement: "" } });
    });

    it("states the backend's count for the holiday that is on", () => {
      render(
        <ChannelSeasonal
          policy={{}}
          onChange={vi.fn()}
          vocabulary={VOCAB}
          holidayEpisodes={[
            { holiday: "halloween", episodes: 9, perDay: 3, everyDays: 3, active: true },
            { holiday: "christmas", episodes: 0, perDay: 0, everyDays: 0, active: false },
          ]}
        />,
      );
      expect(
        screen.getByText(
          "Halloween is on now. 9 Halloween episodes found: 3 a day, each back about every 3 days.",
        ),
      ).toBeInTheDocument();
    });

    it("says so when the next holiday has no episodes in the lineup", () => {
      render(
        <ChannelSeasonal
          policy={{}}
          onChange={vi.fn()}
          vocabulary={VOCAB}
          holidayEpisodes={[{ holiday: "christmas", episodes: 0, perDay: 0, everyDays: 0, active: false }]}
        />,
      );
      expect(
        screen.getByText("Next up: Christmas. No Christmas episodes found in this lineup."),
      ).toBeInTheDocument();
    });
  });
});
