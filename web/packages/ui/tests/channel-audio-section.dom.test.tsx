// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ChannelAudioSection } from "../channels";

afterEach(cleanup);

const OPTIONS = [
  { label: "Auto (default)", value: "__auto__" },
  { label: "English", value: "eng" },
  { label: "French", value: "fra" },
];

describe("ChannelAudioSection", () => {
  it("shows the saved language and hands a new choice back, closing the list", () => {
    const onValueChange = vi.fn();
    const { container } = renderUi(
      <ChannelAudioSection onValueChange={onValueChange} options={OPTIONS} value="eng" />,
    );

    expect(container.querySelector('[role="region"]')?.getAttribute("aria-label")).toBe("Audio");
    const field = container.querySelector<HTMLElement>('[aria-expanded="false"]');
    expect(field?.getAttribute("aria-label")).toBe("Default audio language, English");

    act(() => field?.click());
    const french = Array.from(container.querySelectorAll<HTMLElement>('[role="radio"]')).find((n) =>
      n.textContent?.includes("French"),
    );
    act(() => french?.click());

    expect(onValueChange).toHaveBeenCalledWith("fra");
    expect(container.querySelectorAll('[role="radio"]')).toHaveLength(0);
  });
});
