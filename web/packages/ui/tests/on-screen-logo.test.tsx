// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { OnScreenLogoSection, type OnScreenLogoSectionProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type Node = { click(): void; getAttribute(n: string): string | null; textContent: string | null };
type Root = Parameters<typeof createRoot>[0] & {
  querySelectorAll(s: string): ArrayLike<Node>;
  remove(): void;
  textContent: string | null;
};
const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Root };
  }
).document;

const render = (over: Partial<OnScreenLogoSectionProps> = {}) => {
  const props: OnScreenLogoSectionProps = {
    channelNumber: 7,
    defaults: { look: "text", opacity: 0.4 },
    drawable: true,
    holidays: [
      { id: "halloween", name: "Halloween" },
      { id: "christmas", name: "Christmas" },
    ],
    onChange: vi.fn(),
    onOpenProgramming: vi.fn(),
    onRemoveImage: vi.fn(),
    onUploadImage: vi.fn(),
    policy: {},
    previewSrc: (r) => `/preview?look=${r.look ?? ""}&holiday=${r.holiday ?? ""}`,
    ...over,
  };
  const container = doc.createElement("div");
  doc.body.append(container);
  const root = createRoot(container);
  act(() => {
    root.render(
      <LoomarrProvider theme="dark">
        <OnScreenLogoSection {...props} />
      </LoomarrProvider>,
    );
  });
  const radio = (name: string) => {
    const hit = Array.from(container.querySelectorAll('[role="radio"]')).find(
      (n) => n.getAttribute("aria-label") === name,
    );
    if (!hit) throw new Error(`missing radio: ${name}`);
    return hit;
  };
  return { container, props, radio, unmount: () => act(() => root.unmount()) };
};

describe("OnScreenLogoSection", () => {
  it("saves one field at a time and marks inherited fields Default", () => {
    const { container, props, radio, unmount } = render();
    expect(container.textContent).toContain("On-screen logo");
    expect(container.textContent).toContain("Default");
    expect(container.textContent).not.toContain("Channel override");
    act(() => radio("Bottom left").click());
    expect(props.onChange).toHaveBeenCalledWith({ corner: "bottom-left" });
    act(() => radio("Number badge").click());
    expect(props.onChange).toHaveBeenCalledWith({ look: "badge" });
    unmount();
  });

  it("explains colour does not apply to a white style, and offers swatches for a coloured one", () => {
    const white = render({ policy: { look: "plate" } });
    expect(white.container.textContent).toContain("This style is white, so colour does not apply.");
    white.unmount();
    const coloured = render({ policy: { look: "tint", color: "cyan" } });
    expect(coloured.radio("Cyan").getAttribute("aria-checked")).toBe("true");
    expect(coloured.container.textContent).toContain("Channel override");
    coloured.unmount();
  });

  it("points an Ignore-holidays channel at Programming instead of offering holiday touches", () => {
    const { container, props, unmount } = render({ holidays: [] });
    expect(container.textContent).toContain("This channel ignores holidays");
    const open = Array.from(container.querySelectorAll('[role="button"], button')).find(
      (n) => n.textContent?.trim() === "Open Programming",
    );
    act(() => open?.click());
    expect(props.onOpenProgramming).toHaveBeenCalled();
    expect(container.textContent).not.toContain("Show as");
    unmount();
  });

  it("previews a holiday version from the backend PNG", () => {
    const { container, radio, unmount } = render({ policy: { look: "plate" } });
    act(() => radio("Halloween").click());
    const imgs = Array.from(container.querySelectorAll("img")).map((n) => n.getAttribute("src") ?? "");
    expect(imgs.some((s) => s.includes("holiday=halloween"))).toBe(true);
    unmount();
  });

  it("pauses styles for an own image and warns when the server can't draw", () => {
    const { container, unmount } = render({ policy: { image: "abc" }, drawable: false });
    expect(container.textContent).toContain("Your own image is in use, so styles are paused.");
    expect(container.textContent).toContain("Your own logo airs unchanged, so holiday touches are paused.");
    expect(container.textContent).toContain("This server can't draw the logo right now.");
    unmount();
  });
});
