import type { ChannelPolicy } from "@loomarr/api";
import { render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactElement, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { ChannelPolicyFields } from "./channel-policy-fields";

// Each field's help is a FieldHelp tooltip now, which needs a TooltipProvider ancestor (the
// app mounts one at the root). Wrap every render so the fields mount without a Radix error.
const render = (ui: ReactElement) => rtlRender(<TooltipProvider>{ui}</TooltipProvider>);

const PolicyHarness = ({
  initial,
  onChange,
}: {
  initial: ChannelPolicy;
  onChange: (policy: ChannelPolicy) => void;
}) => {
  const [policy, setPolicy] = useState(initial);
  return (
    <ChannelPolicyFields
      policy={policy}
      onChange={(next) => {
        onChange(next);
        setPolicy(next);
      }}
    />
  );
};

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
  it("adds a movie-release range without changing the independent episode-airing requirement", async () => {
    const onChange = vi.fn();
    render(
      <PolicyHarness
        initial={{
          scope: {
            dates: {
              movieRelease: [{ from: 1990, to: 1999 }],
              seriesAiring: [{ from: 2010, to: 2014 }],
            },
          },
        }}
        onChange={onChange}
      />,
    );

    await userEvent.type(screen.getByLabelText("Movie release new range from"), "2005");
    await userEvent.type(screen.getByLabelText("Movie release new range to"), "2009");
    await userEvent.click(screen.getByRole("button", { name: "Add Movie release range" }));

    expect(onChange).toHaveBeenLastCalledWith({
      scope: {
        dates: {
          movieRelease: [
            { from: 1990, to: 1999 },
            { from: 2005, to: 2009 },
          ],
          seriesAiring: [{ from: 2010, to: 2014 }],
        },
      },
    });
  });

  it("rejects an inverted date edit without clearing the existing range", async () => {
    const onChange = vi.fn();
    render(
      <PolicyHarness
        initial={{ scope: { dates: { movieRelease: [{ from: 1990, to: 1999 }] } } }}
        onChange={onChange}
      />,
    );

    const from = screen.getAllByLabelText("From year")[1]!;
    await userEvent.clear(from);
    await userEvent.type(from, "2005");
    await userEvent.tab();

    expect(screen.getByRole("alert")).toHaveTextContent("From no later than To");
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getAllByLabelText("From year")[1]).toHaveValue(2005);
    expect(screen.getAllByLabelText("To year")[1]).toHaveValue(1999);
  });

  it("commits a recovered date pair without widening the range after an invalid From edit", async () => {
    const onChange = vi.fn();
    render(
      <PolicyHarness
        initial={{
          scope: {
            dates: {
              movieRelease: [
                { from: 1990, to: 1999 },
                { from: 2011, to: 2014 },
              ],
              seriesAiring: [{ from: 1980, to: 1984 }],
            },
          },
        }}
        onChange={onChange}
      />,
    );

    const from = screen.getAllByLabelText("From year")[1]!;
    const to = screen.getAllByLabelText("To year")[1]!;
    await userEvent.clear(from);
    await userEvent.type(from, "2005");
    await userEvent.tab();

    expect(screen.getByRole("alert")).toHaveTextContent("From no later than To");
    expect(onChange).not.toHaveBeenCalled();

    await userEvent.clear(to);
    await userEvent.type(to, "2009");
    await userEvent.tab();

    expect(onChange).toHaveBeenLastCalledWith({
      scope: {
        dates: {
          movieRelease: [
            { from: 2005, to: 2009 },
            { from: 2011, to: 2014 },
          ],
          seriesAiring: [{ from: 1980, to: 1984 }],
        },
      },
    });
    expect(screen.getAllByLabelText("From year")[1]).toHaveValue(2005);
    expect(screen.getAllByLabelText("To year")[1]).toHaveValue(2009);
  });

  it("commits a recovered date pair after an invalid To edit", async () => {
    const onChange = vi.fn();
    render(
      <PolicyHarness
        initial={{ scope: { dates: { movieRelease: [{ from: 1990, to: 1999 }] } } }}
        onChange={onChange}
      />,
    );

    const from = screen.getAllByLabelText("From year")[1]!;
    const to = screen.getAllByLabelText("To year")[1]!;
    await userEvent.clear(to);
    await userEvent.type(to, "1985");
    await userEvent.tab();

    expect(screen.getByRole("alert")).toHaveTextContent("From no later than To");
    expect(onChange).not.toHaveBeenCalled();

    await userEvent.clear(from);
    await userEvent.type(from, "1980");
    await userEvent.tab();

    expect(onChange).toHaveBeenLastCalledWith({
      scope: { dates: { movieRelease: [{ from: 1980, to: 1985 }] } },
    });
  });

  it("removes the dates object only after its last axis is removed", async () => {
    const onChange = vi.fn();
    render(
      <PolicyHarness
        initial={{
          scope: {
            dates: {
              movieRelease: [{ from: 1990, to: 1999 }],
              seriesAiring: [{ from: 2010, to: 2014 }],
            },
          },
        }}
        onChange={onChange}
      />,
    );

    await userEvent.click(screen.getAllByRole("button", { name: "Remove" })[0]!);
    expect(onChange).toHaveBeenLastCalledWith({
      scope: { dates: { movieRelease: undefined, seriesAiring: [{ from: 2010, to: 2014 }] } },
    });

    await userEvent.click(screen.getByRole("button", { name: "Remove" }));
    expect(onChange).toHaveBeenLastCalledWith({ scope: {} });
  });

  it("clears the alternative date representation when switching between era and dates", async () => {
    const onChange = vi.fn();
    render(
      <PolicyHarness
        initial={{
          scope: {
            era: { from: 1980, to: 1989 },
            dates: { movieRelease: [{ from: 1990, to: 1999 }] },
          },
        }}
        onChange={onChange}
      />,
    );

    const scalarFrom = screen.getAllByLabelText("From year")[0]!;
    await userEvent.clear(scalarFrom);
    await userEvent.type(scalarFrom, "1970");
    await userEvent.tab();
    expect(onChange).toHaveBeenLastCalledWith({ scope: { era: { from: 1970, to: 1989 } } });

    await userEvent.type(screen.getByLabelText("Movie release new range from"), "2005");
    await userEvent.type(screen.getByLabelText("Movie release new range to"), "2009");
    await userEvent.click(screen.getByRole("button", { name: "Add Movie release range" }));
    expect(onChange).toHaveBeenLastCalledWith({
      scope: { dates: { movieRelease: [{ from: 2005, to: 2009 }] } },
    });
  });

  it("renders source-explicit fallback and no-limit sentinels for an empty policy", () => {
    render(<ChannelPolicyFields policy={EMPTY} onChange={vi.fn()} />);
    expect(screen.getByRole("combobox", { name: "Play order" })).toHaveTextContent("Use channel strategy");
    expect(screen.getByRole("combobox", { name: "Audience ceiling" })).toHaveTextContent("No limit");
  });

  it("renders the current values of a populated policy", () => {
    render(<ChannelPolicyFields policy={POPULATED} onChange={vi.fn()} />);
    expect(screen.getByRole("combobox", { name: "Play order" })).toHaveTextContent("Shuffled");
    expect(screen.getByRole("combobox", { name: "Audience ceiling" })).toHaveTextContent("TV-14");
    expect(screen.getByLabelText("From year")).toHaveValue(1990);
    expect(screen.getByLabelText("To year")).toHaveValue(1999);
    // Duration strings tidied for display (the wire form the operator reads/types).
    expect(screen.getByLabelText("Same movie")).toHaveValue("168h");
    expect(screen.getByLabelText("Same episode")).toHaveValue("24h");
  });

  it("merges an ordering change into a NEW policy, preserving applied and other sections", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={POPULATED} onChange={onChange} />);

    await userEvent.click(screen.getByRole("combobox", { name: "Play order" }));
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

    await userEvent.click(screen.getByRole("combobox", { name: "Play order" }));
    await userEvent.click(await screen.findByRole("option", { name: "Use channel strategy" }));

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

    const movies = screen.getByLabelText("Same movie");
    await userEvent.type(movies, "168h");
    expect(onChange).not.toHaveBeenCalled();
    await userEvent.tab();

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ separation: { movieNoRepeat: "168h" } }));
  });

  it("clearing a no-repeat field commits undefined (no restriction), not zero", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={POPULATED} onChange={onChange} />);

    const movies = screen.getByLabelText("Same movie");
    await userEvent.clear(movies);
    await userEvent.tab();

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        separation: { movieNoRepeat: undefined, episodeNoRepeat: POPULATED.separation?.episodeNoRepeat },
      }),
    );
  });

  // --- the four doors a surface audit found orphaned (§12 surface map) ---
  //
  // Each of these fields was PATCHable and unreachable: the backend read them, the relaxation
  // ladder narrated them, and no control could set them. A test per door, because a control
  // that renders but does not commit is the same defect wearing a nicer coat.

  it("commits runtimeMax in SECONDS while showing minutes", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={EMPTY} onChange={onChange} />);

    await userEvent.type(screen.getByLabelText("Longest programme"), "90");
    await userEvent.tab();

    // 90 in the box, 5400 on the wire — nobody thinks about programme length in seconds.
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ scope: { runtimeMax: 5400 } }));
  });

  // Clearing must send 0, not undefined: `runtimeMax` is omitempty, so undefined would be
  // dropped from the JSON and the old limit would survive the merge — the field would appear
  // to clear and silently keep filtering.
  it("clears runtimeMax to 0 rather than dropping the field", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={{ scope: { runtimeMax: 5400 } }} onChange={onChange} />);

    await userEvent.clear(screen.getByLabelText("Longest programme"));
    await userEvent.tab();

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ scope: { runtimeMax: 0 } }));
  });

  it("commits a series gap, preserving the no-repeat windows beside it", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={POPULATED} onChange={onChange} />);

    await userEvent.type(screen.getByLabelText("Same series"), "2h");
    await userEvent.tab();

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        separation: expect.objectContaining({
          seriesMinGap: "2h",
          movieNoRepeat: "168h",
          episodeNoRepeat: "24h",
        }),
      }),
    );
  });

  it("commits a block cap as a number", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={EMPTY} onChange={onChange} />);

    await userEvent.type(screen.getByLabelText("Max from one series"), "3");
    await userEvent.tab();

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ separation: expect.objectContaining({ blockMax: 3 }) }),
    );
  });

  // --- unrated allowance: the safety PAIR to the ceiling (§4) ---
  //
  // Orphaned the same way the four above were: the gate has always been enforced and counted in
  // the exclusion report, while nothing could choose it — an operator could read "3 skipped:
  // unrated" with no way to say "allow them".

  it("commits an explicit unrated choice", async () => {
    const onChange = vi.fn();
    render(<ChannelPolicyFields policy={POPULATED} onChange={onChange} />);

    await userEvent.click(screen.getByRole("combobox", { name: "Unrated titles" }));
    await userEvent.click(await screen.findByRole("option", { name: "Allow unrated" }));

    expect(onChange).toHaveBeenCalledWith({
      ...POPULATED,
      audience: { ceiling: "TV-14", unrated: "allow" },
    });
  });

  it("clears unrated back to the automatic sentinel (empty string)", async () => {
    const onChange = vi.fn();
    render(
      <ChannelPolicyFields
        policy={{ audience: { ceiling: "TV-14", unrated: "exclude" } }}
        onChange={onChange}
      />,
    );

    await userEvent.click(screen.getByRole("combobox", { name: "Unrated titles" }));
    await userEvent.click(await screen.findByRole("option", { name: /^Automatic/ }));

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ audience: { ceiling: "TV-14", unrated: "" } }),
    );
  });

  // "Automatic" is not actionable on its own — Go resolves it BY THE CEILING (kids ⇒ exclude,
  // else allow), so the operator cannot tell which way it falls without knowing that rule. The
  // option names its current resolution, and these two pin that it tracks the ceiling above it.
  it("says Automatic currently SKIPS unrated under a kids ceiling", () => {
    render(<ChannelPolicyFields policy={{ audience: { ceiling: "TV-Y7" } }} onChange={vi.fn()} />);
    expect(screen.getByRole("combobox", { name: "Unrated titles" })).toHaveTextContent("Automatic: skipped");
  });

  it("says Automatic currently ALLOWS unrated with no ceiling, and above the kids boundary", () => {
    const { unmount } = render(<ChannelPolicyFields policy={EMPTY} onChange={vi.fn()} />);
    expect(screen.getByRole("combobox", { name: "Unrated titles" })).toHaveTextContent("Automatic: allowed");
    unmount();

    // TV-PG (rank 3) is the LAST kids ceiling; TV-14 (rank 4) is the first that is not.
    render(<ChannelPolicyFields policy={{ audience: { ceiling: "TV-14" } }} onChange={vi.fn()} />);
    expect(screen.getByRole("combobox", { name: "Unrated titles" })).toHaveTextContent("Automatic: allowed");
  });

  it("treats TV-PG as the last kids ceiling (the rank-3 boundary)", () => {
    render(<ChannelPolicyFields policy={{ audience: { ceiling: "TV-PG" } }} onChange={vi.fn()} />);
    expect(screen.getByRole("combobox", { name: "Unrated titles" })).toHaveTextContent("Automatic: skipped");
  });

  // Channel.Strategy is provenance for the unset policy value, not a second editable knob.
  it("names the resolved channel strategy in the one play-order control", () => {
    render(<ChannelPolicyFields policy={EMPTY} onChange={vi.fn()} strategy="sequential" />);

    expect(screen.getByRole("combobox", { name: "Play order" })).toHaveTextContent(
      "Use channel strategy (In order)",
    );
    expect(screen.queryByLabelText("Playback")).not.toBeInTheDocument();
  });

  it("says blank repeat fields use built-in spacing", () => {
    render(<ChannelPolicyFields policy={EMPTY} onChange={vi.fn()} show="ordering" />);

    expect(screen.getByLabelText("Same movie")).toHaveAttribute("placeholder", "Use built-in");
    expect(screen.getByLabelText("Same episode")).toHaveAttribute("placeholder", "Use built-in");
    expect(screen.getByLabelText("Same series")).toHaveAttribute("placeholder", "Use built-in");
    expect(screen.getByLabelText("Max from one series")).toHaveAttribute("placeholder", "Use built-in");
  });
});
