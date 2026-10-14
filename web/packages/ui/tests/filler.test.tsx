// @vitest-environment jsdom
import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ClipSelect, NeedsDetailsChip } from "../index";

// The package type-checks without the DOM lib, so the test names only the node surface it reads.
type BoxNode = { click(): void; getAttribute(name: string): string | null };
type Host = Parameters<typeof createRoot>[0] & {
  querySelector(selector: string): BoxNode | null;
  remove(): void;
  textContent: string | null;
};
const testDocument = (
  globalThis as unknown as {
    document: { body: { append(node: unknown): void }; createElement(tagName: string): Host };
  }
).document;

const render = (node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

describe("NeedsDetailsChip", () => {
  it("names the exception", () => {
    expect(render(<NeedsDetailsChip />)).toContain("Needs details");
  });
});

describe("ClipSelect", () => {
  let host: Host;
  let root: Root;
  const mount = (node: React.ReactNode) => {
    act(() => root.render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>));
    const box = host.querySelector('[role="checkbox"]');
    if (!box) throw new Error("no checkbox rendered");
    return box;
  };

  beforeEach(() => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    host = testDocument.createElement("div");
    testDocument.body.append(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
  });

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
    const style = box.getAttribute("style") ?? "";
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
