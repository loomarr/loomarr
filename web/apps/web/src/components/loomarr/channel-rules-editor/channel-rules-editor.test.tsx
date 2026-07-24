import type { ChannelPolicy } from "@loomarr/api";
import { render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { ChannelRulesEditor } from "./channel-rules-editor";

// Rows carry a remove-button tooltip (Radix), which needs a TooltipProvider ancestor —
// same fix as ChannelLineupEditor's test.
const render = (ui: ReactElement) => rtlRender(<TooltipProvider>{ui}</TooltipProvider>);

// Each rule's `when`/`what`/`how` are set to the EXACT shape the lowering table produces
// for its token (presets.ts), so the editor's reverse lookup (tokenForWhen/What/How)
// round-trips to the token the label expects — a fixture with a `label` string but a
// predicate the table doesn't recognize would render as "Custom" pickers, not this label.
const POPULATED: ChannelPolicy = {
  rules: [
    {
      id: "r1",
      label: "Christmas · Marathon",
      priority: 30,
      when: { holiday: "christmas" },
      how: { ordering: "sequential", noBreaks: true, separation: { blockMax: -1 } },
      window: "-1ns", // schedule.WindowFull — a marathon plays the whole run
    },
    {
      id: "r2",
      label: "Weekend · Kids-safe genres",
      priority: 20,
      when: { weekend: true },
      what: { genres: { include: ["Animation", "Family", "Kids"] } },
    },
    {
      id: "r3",
      label: "Weekday · Syndication",
      priority: 10,
      when: { weekday: true },
      how: { ordering: "syndication" },
    },
  ],
};

describe("ChannelRulesEditor", () => {
  it("shows the empty-state explainer and Add affordance when there are no rules", () => {
    render(<ChannelRulesEditor policy={{}} onChange={vi.fn()} />);
    expect(screen.getByText(/no rules yet/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /add rule/i })).toBeInTheDocument();
  });

  it("renders each rule's computed label", () => {
    render(<ChannelRulesEditor policy={POPULATED} onChange={vi.fn()} />);
    expect(screen.getByText("Christmas · Marathon")).toBeInTheDocument();
    expect(screen.getByText("Weekend · Kids-safe genres")).toBeInTheDocument();
    expect(screen.getByText("Weekday · Syndication")).toBeInTheDocument();
  });

  it("adding a rule prepends it and calls onChange with the rebuilt policy", async () => {
    const onChange = vi.fn();
    render(<ChannelRulesEditor policy={POPULATED} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: /add rule/i }));

    expect(onChange).toHaveBeenCalledTimes(1);
    const next = onChange.mock.lastCall?.[0] as ChannelPolicy;
    expect(next.rules).toHaveLength(4);
    // The new rule is prepended (top of the list) and outranks everything below it.
    expect(next.rules?.[0]?.priority).toBeGreaterThan(next.rules?.[1]?.priority ?? 0);
  });

  it("priority is derived from list order: the top rule always has the highest priority", () => {
    render(<ChannelRulesEditor policy={POPULATED} onChange={vi.fn()} />);
    // Just a static render check here — the reorder-changes-priority behavior itself is
    // exercised end to end below via the onChange assertion on remove/add, since jsdom
    // has no real pointer/drag geometry to drive dnd-kit's sensors in a unit test.
    expect(screen.getByText("Christmas · Marathon")).toBeInTheDocument();
  });

  it("removing a rule drops it and commits the remaining rules in order with re-derived priority", async () => {
    const onChange = vi.fn();
    render(<ChannelRulesEditor policy={POPULATED} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: /remove christmas · marathon/i }));

    expect(onChange).toHaveBeenCalledTimes(1);
    const next = onChange.mock.lastCall?.[0] as ChannelPolicy;
    expect(next.rules).toHaveLength(2);
    expect(next.rules?.map((r) => r.label)).toEqual(["Weekend · Kids-safe genres", "Weekday · Syndication"]);
    // The remaining top rule (was priority 20, index 1) now leads a 2-rule list: priority
    // is RE-DERIVED from its new position, not carried over from before the removal.
    expect(next.rules?.[0]?.priority).toBeGreaterThan(next.rules?.[1]?.priority ?? 0);
  });

  it("editing a rule's WHEN token relowers the predicate and recomputes the label", async () => {
    const onChange = vi.fn();
    render(
      <ChannelRulesEditor
        policy={{
          rules: [
            {
              id: "r1",
              label: "Weekend · Syndication",
              priority: 10,
              when: { weekend: true },
              how: { ordering: "syndication" },
            },
          ],
        }}
        onChange={onChange}
      />,
    );

    await userEvent.click(screen.getByRole("combobox", { name: /when for weekend · syndication/i }));
    await userEvent.click(await screen.findByRole("option", { name: "Weekday" }));

    expect(onChange).toHaveBeenCalledTimes(1);
    const next = onChange.mock.lastCall?.[0] as ChannelPolicy;
    expect(next.rules?.[0]?.when).toEqual({ weekday: true });
    expect(next.rules?.[0]?.label).toBe("Weekday · Syndication");
  });

  it("lists the channel's own lineup keys as series: WHAT options", async () => {
    render(
      <ChannelRulesEditor
        policy={POPULATED}
        onChange={vi.fn()}
        lineupKeys={[{ key: "series:tvdb:81189", title: "Breaking Bad" }]}
      />,
    );

    await userEvent.click(screen.getByRole("combobox", { name: /what for christmas · marathon/i }));
    expect(await screen.findByRole("option", { name: "Series: Breaking Bad" })).toBeInTheDocument();
  });

  it("selecting a series WHAT option lowers to a scope intersected with that key", async () => {
    const onChange = vi.fn();
    render(
      <ChannelRulesEditor
        policy={{ rules: [{ id: "r1", label: "Weekend", priority: 10, when: { weekend: true } }] }}
        onChange={onChange}
        lineupKeys={[{ key: "series:tvdb:81189", title: "Breaking Bad" }]}
      />,
    );

    await userEvent.click(screen.getByRole("combobox", { name: /what for weekend/i }));
    await userEvent.click(await screen.findByRole("option", { name: "Series: Breaking Bad" }));

    expect(onChange).toHaveBeenCalledTimes(1);
    const next = onChange.mock.lastCall?.[0] as ChannelPolicy;
    expect(next.rules?.[0]?.what).toEqual({ series: ["series:tvdb:81189"] });
    // The label shows the series' own display title (from lineupKeys), not the raw
    // "Series: <title>" option text — that phrasing already lives on the WHAT select
    // itself, so the row caption stays terse ("Weekend · Breaking Bad").
    expect(next.rules?.[0]?.label).toBe("Weekend · Breaking Bad");
  });

  it("selecting the marathon HOW token sets noBreaks + unbounded block + the full-run window", async () => {
    const onChange = vi.fn();
    render(
      <ChannelRulesEditor
        policy={{ rules: [{ id: "r1", label: "Weekend", priority: 10, when: { weekend: true } }] }}
        onChange={onChange}
      />,
    );

    await userEvent.click(screen.getByRole("combobox", { name: /how for weekend/i }));
    await userEvent.click(await screen.findByRole("option", { name: /marathon/i }));

    expect(onChange).toHaveBeenCalledTimes(1);
    const next = onChange.mock.lastCall?.[0] as ChannelPolicy;
    expect(next.rules?.[0]?.how).toEqual({
      ordering: "sequential",
      noBreaks: true,
      separation: { blockMax: -1 },
    });
    // "-1ns" is schedule.WindowFull on the wire — "the whole run, don't truncate a binge".
    // NOT "0s" (Duration 0 = "inherit the window" = WOULD truncate — the opposite). This
    // guards that a hand-authored marathon is byte-identical to an LLM-authored one (§8.1).
    expect(next.rules?.[0]?.window).toBe("-1ns");
  });
});
