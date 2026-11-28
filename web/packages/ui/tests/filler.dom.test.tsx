// @vitest-environment jsdom
import { LoomarrProvider } from "@loomarr/design-system";
import { act, cleanup, renderUi } from "@loomarr/test-support";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ClipSelect, NeedsDetailsChip } from "../filler";

const render = (node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

describe("NeedsDetailsChip", () => {
  it("names the exception", () => {
    expect(render(<NeedsDetailsChip />)).toContain("Needs details");
  });
});

describe("ClipSelect", () => {
  let host: HTMLElement;
  let view: ReturnType<typeof renderUi>;
  const mount = (node: React.ReactNode) => {
    view.rerender(node);
    const box = host.querySelector<HTMLElement>('[role="checkbox"]');
    if (!box) throw new Error("no checkbox rendered");
    return box;
  };

  beforeEach(() => {
    view = renderUi(null);
    host = view.container;
  });

  afterEach(cleanup);

  it("is a checkbox named for the clip that prints no label", () => {
    const box = mount(<ClipSelect clipName="Sunbeam Juice" onSelectedChange={() => {}} selected={false} />);
    expect(box.getAttribute("aria-label")).toBe("Select Sunbeam Juice");
    expect(box.getAttribute("aria-checked")).toBe("false");
    expect(host.textContent).toBe("");
  });

  // WCAG 2.2, 2.5.8: the Library's decision 7 (#1908) moved this off a 16px box. The design-system
  // checkbox is a 24px box in a 40px-tall pressable at pointer density.
  it("gives the select box a target of at least 24px", () => {
    const box = mount(<ClipSelect clipName="Sunbeam Juice" onSelectedChange={() => {}} selected={false} />);
    // On the web the press target is the <label> around the box, which carries the row's height.
    const style = (box.closest("label") ?? box).getAttribute("style") ?? "";
    const minHeight = Number(/min-height:\s*(\d+)px/.exec(style)?.[1]);
    expect(minHeight).toBeGreaterThanOrEqual(24);
  });

  it("reports the flipped state on press", () => {
    const onSelectedChange = vi.fn();
    const box = mount(<ClipSelect clipName="Sunbeam Juice" onSelectedChange={onSelectedChange} selected />);
    expect(box.getAttribute("aria-checked")).toBe("true");
    act(() => box.click());
    expect(onSelectedChange).toHaveBeenCalledWith(false);
  });
});
