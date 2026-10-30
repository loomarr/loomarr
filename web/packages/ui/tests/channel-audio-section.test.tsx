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

const SUBTITLE_OPTIONS = [
  { label: "Off", value: "off" },
  { label: "English", value: "eng" },
];

const mount = (onValueChange: (v: string) => void, onSubtitles: (v: string) => void) => {
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() =>
    root.render(
      <LoomarrProvider theme="dark">
        <ChannelAudioSection
          onValueChange={onValueChange}
          options={OPTIONS}
          subtitles={{ onValueChange: onSubtitles, options: SUBTITLE_OPTIONS, value: "off" }}
          value="eng"
        />
      </LoomarrProvider>,
    ),
  );
  return { host, root };
};

describe("ChannelAudioSection", () => {
  it("shows the saved language and hands a new choice back, closing the list", () => {
    const onValueChange = vi.fn();
    const { host, root } = mount(onValueChange, vi.fn());

    expect(host.querySelector('[role="region"]')?.getAttribute("aria-label")).toBe("Audio and subtitles");
    const field = host.querySelector('[aria-label="Default audio language, English"]');
    expect(field?.getAttribute("aria-expanded")).toBe("false");

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

  it("has the burned-in subtitles picker under the audio one, with its own choices", () => {
    const onSubtitles = vi.fn();
    const { host, root } = mount(vi.fn(), onSubtitles);

    const field = host.querySelector('[aria-label="Burned-in subtitles, Off"]');
    act(() => field?.click());
    const english = Array.from(host.querySelectorAll('[role="radio"]')).find((n) =>
      n.textContent?.includes("English"),
    );
    act(() => english?.click());

    expect(onSubtitles).toHaveBeenCalledWith("eng");
    act(() => root.unmount());
    host.remove();
  });
});
