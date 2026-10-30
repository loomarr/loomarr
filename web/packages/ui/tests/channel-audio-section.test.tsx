// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { ChannelAudioSection } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type Node = { click(): void; getAttribute(n: string): string | null; textContent: string | null };
type Root = Parameters<typeof createRoot>[0] & {
  querySelector(s: string): Node | null;
  querySelectorAll(s: string): ArrayLike<Node>;
  remove(): void;
};
const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Root };
  }
).document;

const OPTIONS = [
  { label: "Auto (default)", value: "__auto__" },
  { label: "English", value: "eng" },
  { label: "French", value: "fra" },
];

describe("ChannelAudioSection", () => {
  it("shows the saved language and hands a new choice back, closing the list", () => {
    const onValueChange = vi.fn();
    const host = doc.createElement("div");
    doc.body.append(host);
    const root = createRoot(host);
    act(() =>
      root.render(
        <LoomarrProvider theme="dark">
          <ChannelAudioSection onValueChange={onValueChange} options={OPTIONS} value="eng" />
        </LoomarrProvider>,
      ),
    );

    expect(host.querySelector('[role="region"]')?.getAttribute("aria-label")).toBe("Audio");
    const field = host.querySelector('[aria-expanded="false"]');
    expect(field?.getAttribute("aria-label")).toBe("Default audio language, English");

    act(() => field?.click());
    const french = Array.from(host.querySelectorAll('[role="radio"]')).find((n) =>
      n.textContent?.includes("French"),
    );
    act(() => french?.click());

    expect(onValueChange).toHaveBeenCalledWith("fra");
    expect(host.querySelectorAll('[role="radio"]')).toHaveLength(0);
    act(() => root.unmount());
    host.remove();
  });
});
