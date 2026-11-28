// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AiReconnectBanner } from "../index";

const mount = async (node: ReactNode) => {
  await act(async () => {
    renderUi(node);
  });
};
afterEach(cleanup);

const text = () => document.body.textContent ?? "";
const button = (name: string) => {
  const found = Array.from(document.body.querySelectorAll<HTMLElement>('[role="button"]')).find(
    (node) => node.textContent?.trim() === name,
  );
  if (!found) throw new Error(`missing button: ${name}`);
  return found;
};

describe("AiReconnectBanner", () => {
  it("says the mock's copy, with the picks counted, as a polite status", async () => {
    await mount(<AiReconnectBanner pickCount={1} onKeepManual={vi.fn()} onTryAi={vi.fn()} />);

    expect(document.body.querySelector('[role="status"]')?.getAttribute("aria-live")).toBe("polite");
    expect(text()).toContain("AI is connected now");
    expect(text()).toContain(
      "Your manual lineup (1 title) is still here. Keep building it by hand, or let Loomarr suggest titles for this description — your picks stay in the lineup either way.",
    );
  });

  it("offers both actions", async () => {
    const onTryAi = vi.fn();
    const onKeepManual = vi.fn();
    await mount(<AiReconnectBanner pickCount={3} onKeepManual={onKeepManual} onTryAi={onTryAi} />);

    await act(async () => button("Try AI suggestions").click());
    await act(async () => button("Keep building manually").click());

    expect(onTryAi).toHaveBeenCalledOnce();
    expect(onKeepManual).toHaveBeenCalledOnce();
  });

  it("holds Try AI suggestions and says why when it can't run yet", async () => {
    const onTryAi = vi.fn();
    await mount(
      <AiReconnectBanner
        pickCount={2}
        onKeepManual={vi.fn()}
        onTryAi={onTryAi}
        tryAiBlockedReason="Name this channel to try AI suggestions."
      />,
    );
    const tryAi = button("Try AI suggestions");

    await act(async () => tryAi.click());

    expect(tryAi.getAttribute("aria-disabled")).toBe("true");
    expect(onTryAi).not.toHaveBeenCalled();
    expect(text()).toContain("Name this channel to try AI suggestions.");
  });
});
