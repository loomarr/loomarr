import type { ChannelPolicy } from "@loomarr/api";
import { render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { ChannelAutoCurate } from "./channel-auto-curate";

// The opt-in's help is a FieldHelp tooltip, which needs a TooltipProvider ancestor.
const render = (ui: ReactElement) => rtlRender(<TooltipProvider>{ui}</TooltipProvider>);

const OPT_IN = "Add new titles without asking";

// Carries `applied` (reconcile-owned, §9) so every test can assert it survives untouched, and
// a scope so the tests prove unrelated sections are not rebuilt by an auto-curate edit.
const POPULATED: ChannelPolicy = {
  ordering: "shuffle",
  scope: { era: { from: 1990, to: 1999 } },
  applied: [{ kind: "blockMax", from: "8", to: "unbounded" }],
};

describe("ChannelAutoCurate", () => {
  it("is off when the policy carries no autoCurate object", () => {
    render(<ChannelAutoCurate policy={POPULATED} onChange={vi.fn()} />);
    expect(screen.getByLabelText(OPT_IN)).not.toBeChecked();
  });

  // THE structural case (programming-design.md §8.2): the opt-in IS the object's presence, so a
  // zero-value {} means "opted in, inherit both thresholds" — NOT "off". This is why no generic
  // policy field editor could ever reach it: there is no boolean to bind to.
  it("is on for a zero-value autoCurate object", () => {
    render(<ChannelAutoCurate policy={{ ...POPULATED, autoCurate: {} }} onChange={vi.fn()} />);
    expect(screen.getByLabelText(OPT_IN)).toBeChecked();
  });

  it("opting in CONSTRUCTS the object, preserving every other section", async () => {
    const onChange = vi.fn();
    render(<ChannelAutoCurate policy={POPULATED} onChange={onChange} />);

    await userEvent.click(screen.getByLabelText(OPT_IN));

    expect(onChange).toHaveBeenCalledWith({ ...POPULATED, autoCurate: {} });
    expect(onChange.mock.lastCall?.[0].applied).toBe(POPULATED.applied);
  });

  // Opting out must REMOVE the key, not set it to a falsy object. `{}` would read as opted-in
  // (the case above), and the channel would keep auto-approving acquisitions after the operator
  // switched it off — a safety-adjacent failure, since auto-curate spends acquisitions
  // unattended. Safe to remove because PATCH replaces the policy wholesale
  // (schedule.MergeFromOperator: `out := incoming`), so an absent key genuinely clears it
  // rather than reading as "unchanged".
  it("opting out REMOVES the object rather than emptying it", async () => {
    const onChange = vi.fn();
    render(
      <ChannelAutoCurate policy={{ ...POPULATED, autoCurate: { maxTitles: 40 } }} onChange={onChange} />,
    );

    await userEvent.click(screen.getByLabelText(OPT_IN));

    const next = onChange.mock.lastCall?.[0];
    expect(next).not.toHaveProperty("autoCurate");
    expect(next.scope).toEqual(POPULATED.scope);
    expect(next.applied).toBe(POPULATED.applied);
  });

  // The hint must describe the CURRENT state. A first cut always read "Off, new titles wait for
  // your approval" — which contradicted a ticked box. Every assertion above still passed (the
  // checkbox state and the committed payload were right); only the story screenshot showed it.
  // Pinned in both directions so the copy cannot drift back out of sync with the control.
  it("states what is happening now, in both directions", () => {
    const { unmount } = render(<ChannelAutoCurate policy={POPULATED} onChange={vi.fn()} />);
    expect(screen.getByText(/wait for your approval/)).toBeInTheDocument();
    unmount();

    render(<ChannelAutoCurate policy={{ ...POPULATED, autoCurate: {} }} onChange={vi.fn()} />);
    expect(screen.getByText(/added on their own/)).toBeInTheDocument();
    expect(screen.queryByText(/wait for your approval/)).not.toBeInTheDocument();
  });

  it("hides the threshold overrides until opted in", () => {
    const { unmount } = render(<ChannelAutoCurate policy={POPULATED} onChange={vi.fn()} />);
    expect(screen.queryByLabelText("Quality bar")).not.toBeInTheDocument();
    unmount();

    render(<ChannelAutoCurate policy={{ ...POPULATED, autoCurate: {} }} onChange={vi.fn()} />);
    expect(screen.getByLabelText("Quality bar")).toBeInTheDocument();
    expect(screen.getByLabelText("Title cap")).toBeInTheDocument();
  });

  // 0 is the INHERIT sentinel on the wire, so a stored 0 must render as an empty box reading
  // "Default" — not a literal 0, which would make inherit look like an explicit zero (and, for
  // the quality bar, like "accept anything").
  it("renders an inherited threshold as blank, not as 0", () => {
    render(
      <ChannelAutoCurate policy={{ autoCurate: { minScorePct: 0, maxTitles: 0 } }} onChange={vi.fn()} />,
    );
    expect(screen.getByLabelText("Quality bar")).toHaveValue(null);
    expect(screen.getByLabelText("Title cap")).toHaveValue(null);
  });

  it("commits a threshold override on blur, not on keystroke", async () => {
    const onChange = vi.fn();
    render(<ChannelAutoCurate policy={{ ...POPULATED, autoCurate: {} }} onChange={onChange} />);

    await userEvent.type(screen.getByLabelText("Quality bar"), "75");
    expect(onChange).not.toHaveBeenCalled();
    await userEvent.tab();

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ autoCurate: { minScorePct: 75 } }));
  });

  // Clearing sends 0 (inherit), never undefined: minScorePct/maxTitles are omitempty ints, so
  // undefined would be dropped from the JSON and the previous override would survive the merge
  // — the field would appear to clear and silently keep applying. Same trap as runtimeMax.
  it("clearing a threshold commits 0 (inherit) rather than dropping the field", async () => {
    const onChange = vi.fn();
    render(<ChannelAutoCurate policy={{ autoCurate: { minScorePct: 75 } }} onChange={onChange} />);

    await userEvent.clear(screen.getByLabelText("Quality bar"));
    await userEvent.tab();

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ autoCurate: { minScorePct: 0 } }));
  });

  it("keeps the other threshold when one is edited", async () => {
    const onChange = vi.fn();
    render(<ChannelAutoCurate policy={{ autoCurate: { minScorePct: 75 } }} onChange={onChange} />);

    await userEvent.type(screen.getByLabelText("Title cap"), "40");
    await userEvent.tab();

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ autoCurate: { minScorePct: 75, maxTitles: 40 } }),
    );
  });

  // §8.2 skips channels with no IntentRef — re-curation re-runs the channel's stored intent, so
  // a hand-made channel has nothing to re-evaluate. Offering an enabled checkbox there would
  // save a setting the job can never act on: a control that lies about what it does.
  it("is disabled, and explains why, for a hand-made channel", () => {
    render(<ChannelAutoCurate policy={POPULATED} onChange={vi.fn()} intentBacked={false} />);

    expect(screen.getByLabelText(OPT_IN)).toBeDisabled();
    expect(screen.getByText(/made by hand/)).toBeInTheDocument();
  });
});
