// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AiReconnectBanner } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const mount = async (node: ReactNode) => {
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);
  await act(async () => {
    root.render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);
  });
  return container;
};
afterEach(() => {
  document.body.replaceChildren();
});

const button = (container: HTMLElement, name: string) => {
  const found = Array.from(container.querySelectorAll('[role="button"]')).find(
    (node) => node.textContent?.trim() === name,
  );
  if (!found) throw new Error(`missing button: ${name}`);
  return found as HTMLElement;
};

describe("AiReconnectBanner", () => {
  it("says the mock's copy, with the picks counted, as a polite status", async () => {
    const container = await mount(
      <AiReconnectBanner pickCount={1} onKeepManual={vi.fn()} onTryAi={vi.fn()} />,
    );

    expect(container.querySelector('[role="status"]')?.getAttribute("aria-live")).toBe("polite");
    expect(container.textContent).toContain("AI is connected now");
    expect(container.textContent).toContain(
      "Your manual lineup (1 title) is still here. Keep building it by hand, or let Loomarr suggest titles for this description — your picks stay in the lineup either way.",
    );
  });

  it("offers both actions", async () => {
    const onTryAi = vi.fn();
    const onKeepManual = vi.fn();
    const container = await mount(
      <AiReconnectBanner pickCount={3} onKeepManual={onKeepManual} onTryAi={onTryAi} />,
    );

    await act(async () => button(container, "Try AI suggestions").click());
    await act(async () => button(container, "Keep building manually").click());

    expect(onTryAi).toHaveBeenCalledOnce();
    expect(onKeepManual).toHaveBeenCalledOnce();
  });

  it("holds Try AI suggestions and says why when it can't run yet", async () => {
    const onTryAi = vi.fn();
    const container = await mount(
      <AiReconnectBanner
        pickCount={2}
        onKeepManual={vi.fn()}
        onTryAi={onTryAi}
        tryAiBlockedReason="Name this channel to try AI suggestions."
      />,
    );
    const tryAi = button(container, "Try AI suggestions");

    await act(async () => tryAi.click());

    expect(tryAi.getAttribute("aria-disabled")).toBe("true");
    expect(onTryAi).not.toHaveBeenCalled();
    expect(container.textContent).toContain("Name this channel to try AI suggestions.");
  });
});
