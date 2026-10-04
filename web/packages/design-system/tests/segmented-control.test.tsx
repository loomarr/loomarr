import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { LoomarrProvider, SegmentedControl } from "../index";

const render = (node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

const options = [
  { count: 6, label: "Needs you", value: "needs-you" },
  { count: 3, label: "In progress", value: "in-progress" },
  { count: 2, label: "Done", value: "done" },
] as const;

const control = (over: Partial<Parameters<typeof SegmentedControl>[0]> = {}) =>
  render(
    <SegmentedControl
      accessibilityLabel="Requests sections"
      onValueChange={vi.fn()}
      options={options}
      value="in-progress"
      {...over}
    />,
  );

describe("segmented control", () => {
  it("is one labelled tab list with a segment per option and one selected", () => {
    const markup = control();
    expect(markup).toContain('role="tablist"');
    expect(markup).toContain('aria-label="Requests sections"');
    expect(markup.match(/role="tab"/g)).toHaveLength(3);
    expect(markup.match(/aria-selected="true"/g)).toHaveLength(1);
    expect(markup.match(/aria-selected="false"/g)).toHaveLength(2);
  });

  it("puts the count in each segment's accessible label as well as beside its text", () => {
    const markup = control();
    expect(markup).toContain('aria-label="Needs you, 6"');
    expect(markup).toContain('aria-label="In progress, 3"');
    expect(markup).toContain('aria-label="Done, 2"');
    expect(markup).toContain(">Needs you<");
    expect(markup).toContain(">6<");
  });

  it("says only the label when a segment has no count", () => {
    const markup = control({ options: [{ label: "Done", value: "done" }], value: "done" });
    expect(markup).toContain('aria-label="Done"');
    expect(markup).not.toContain("Done,");
  });

  it("keeps a zero count, which is a count", () => {
    expect(control({ options: [{ count: 0, label: "Done", value: "done" }], value: "done" })).toContain(
      'aria-label="Done, 0"',
    );
  });

  it("makes every segment at least 44 pt tall and gives each an equal share of the width", () => {
    const markup = control();
    expect(markup.match(/style="flex:1;min-height:44px"/g)).toHaveLength(3);
  });

  it("draws two segments for two options", () => {
    const markup = control({ options: options.slice(1) });
    expect(markup.match(/role="tab"/g)).toHaveLength(2);
  });

  it("marks a disabled segment as disabled to a screen reader", () => {
    const markup = control({ options: [{ ...options[0], disabled: true }, options[1]] });
    expect(markup).toContain('aria-disabled="true"');
  });
});
