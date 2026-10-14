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
    everyHoliday: false,
    holidays: [
      {
        id: "halloween",
        name: "Halloween",
        start: "2026-10-01",
        end: "2026-10-31",
        active: true,
        picture: "pumpkin",
        pictures: [
          { id: "pumpkin", name: "Pumpkin" },
          { id: "bat", name: "Bat" },
          { id: "ghost", name: "Ghost" },
        ],
      },
      {
        id: "christmas",
        name: "Christmas",
        start: "2026-12-01",
        end: "2026-12-26",
        active: false,
        picture: "holly",
        pictures: [
          { id: "holly", name: "Holly" },
          { id: "snowflake", name: "Snowflake" },
        ],
      },
    ],
    today: new Date(2026, 9, 13),
    onChange: vi.fn(),
    onOpenProgramming: vi.fn(),
    onRemoveImage: vi.fn(),
    onUploadImage: vi.fn(),
    policy: {},
    previewSrc: vi.fn((r) => `/preview?look=${r.look ?? ""}&holiday=${r.holiday ?? ""}`),
    ...over,
  };
  const container = doc.createElement("div");
  doc.body.append(container);
  const root = createRoot(container);
  const rerender = (next: Partial<OnScreenLogoSectionProps>) =>
    act(() => {
      root.render(
        <LoomarrProvider theme="dark">
          <OnScreenLogoSection {...props} {...next} />
        </LoomarrProvider>,
      );
    });
  rerender({});
  const radio = (name: string) => {
    const hit = Array.from(container.querySelectorAll('[role="radio"]')).find(
      (n) => n.getAttribute("aria-label") === name,
    );
    if (!hit) throw new Error(`missing radio: ${name}`);
    return hit;
  };
  return { container, props, radio, rerender, unmount: () => act(() => root.unmount()) };
};

describe("OnScreenLogoSection", () => {
  it("re-syncs the name box when the saved name is reset, and a blur then saves nothing", () => {
    const { container, props, rerender } = render({ policy: { callsign: "WXYZ" } });
    const input = () =>
      container.querySelectorAll('[aria-label="Name on the logo"]')[0] as unknown as {
        dispatchEvent(e: unknown): void;
        value: string;
      };
    expect(input().value).toBe("WXYZ");
    rerender({ policy: {} });
    expect(input().value).toBe("");
    act(() => input().dispatchEvent(new FocusEvent("focusout", { bubbles: true })));
    expect(props.onChange).not.toHaveBeenCalled();
  });

  it("is one tab stop per group and the arrow keys move the style choice", () => {
    const { container, props, radio } = render({ policy: { look: "text" } });
    const stops = (name: string) =>
      Array.from(
        container.querySelectorAll(`[role="radiogroup"][aria-label="${name}"] [role="radio"]`),
      ).filter((n) => n.getAttribute("tabindex") === "0");
    expect(stops("Position")).toHaveLength(1);
    const first = radio("Top left") as unknown as { dispatchEvent(e: unknown): void };
    act(() => first.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, key: "ArrowRight" })));
    expect(props.onChange).toHaveBeenCalledWith({ corner: "top-right" });
  });

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

  it("previews today's holiday version from the backend PNG, and the everyday logo on request", () => {
    const { container, radio, unmount } = render({ policy: { look: "plate" } });
    const preview = () =>
      Array.from(container.querySelectorAll('[data-testid="logo-preview"] img')).map((n) =>
        n.getAttribute("src"),
      );
    expect(preview().every((s) => s?.includes("holiday=halloween"))).toBe(true);
    act(() => radio("Everyday").click());
    expect(preview().every((s) => s?.endsWith("holiday="))).toBe(true);
    unmount();
  });

  it("lists each holiday with its dates, marks the one in effect, and says what airs when", () => {
    const { container, props, radio, unmount } = render();
    const text = container.textContent ?? "";
    expect(text).toContain("Oct 1 – Oct 31");
    expect(text).toContain("Dec 1 – Dec 26");
    expect(text.match(/In effect now/g)).toHaveLength(1);
    expect(text).toContain("it uses Halloween, Christmas");
    expect(text).toContain("From Oct 1 to Oct 31 this channel shows the Halloween logo.");
    expect(text).toContain("Today (10/13) that is the Halloween one.");
    expect(text).toContain("Text with a Halloween pumpkin");
    expect(radio("Pumpkin for Halloween").getAttribute("aria-checked")).toBe("true");
    act(() => radio("Bat for Halloween").click());
    expect(props.onChange).toHaveBeenCalledWith({ motifs: { halloween: "bat" } });
    // Each chip's picture is the backend's picture-only PNG, never a web drawing.
    expect(props.previewSrc).toHaveBeenCalledWith({ holiday: "halloween", motifOnly: true, picture: "bat" });
    unmount();
  });

  it("names the era's palette, or amber without one, and shows the automatic name", () => {
    const era = render({ autoCallsign: "SC", eraLabel: "1980s", policy: { look: "tint" } });
    expect(era.container.textContent).toContain("1980s palette for this channel.");
    expect(era.container.querySelectorAll('input[placeholder="Auto: SC"]')).toHaveLength(1);
    era.unmount();
    const none = render({ policy: { look: "tint" } });
    expect(none.container.textContent).toContain("no era on its Programming tab, so it uses amber");
    none.unmount();
  });

  it("pauses styles for an own image and warns when the server can't draw", () => {
    const { container, unmount } = render({ policy: { image: "abc" }, drawable: false });
    expect(container.textContent).toContain("Your own image is in use, so styles are paused.");
    expect(container.textContent).toContain("Your own logo airs unchanged, so holiday touches are paused.");
    expect(container.textContent).toContain("This server can't draw the logo right now.");
    unmount();
  });

  it("shows the uploaded image as a thumbnail with a plain label and a Remove button", () => {
    const onRemoveImage = vi.fn();
    const { container, unmount } = render({
      imageSrc: "/v1/images/abc/w185.webp",
      onRemoveImage,
      policy: { image: "abc" },
    });
    const thumb = Array.from(container.querySelectorAll('img[src="/v1/images/abc/w185.webp"]'));
    expect(thumb).toHaveLength(1);
    expect(container.textContent).toContain("Your own image");
    expect(container.textContent).not.toContain("Your image is in use.");
    const remove = Array.from(container.querySelectorAll('[role="button"], button')).find(
      (n) => n.textContent === "Remove",
    );
    act(() => remove?.click());
    expect(onRemoveImage).toHaveBeenCalledTimes(1);
    unmount();
  });
});
