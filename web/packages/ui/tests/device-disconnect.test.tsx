// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { AccessibilityInfo } from "react-native";
import { describe, expect, it, vi } from "vitest";

import { DeviceDisconnectAction } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type ClickableNode = { click(): void; textContent?: string | null };
type QueryRoot = { querySelectorAll(selector: string): ArrayLike<ClickableNode> };
const testDocument = (
  globalThis as unknown as {
    document: QueryRoot & {
      body: QueryRoot & { append(node: unknown): void; textContent?: string | null };
      createElement(
        tagName: string,
      ): Parameters<typeof createRoot>[0] & QueryRoot & { remove(): void; textContent?: string | null };
    };
  }
).document;
const buttonNamed = (container: QueryRoot, name: string) => {
  const button = Array.from(container.querySelectorAll('[role="button"], button')).find(
    (candidate) => candidate.textContent?.trim() === name,
  );
  if (!button) throw new Error(`missing button: ${name}`);
  return button;
};

describe("DeviceDisconnectAction", () => {
  it("offers an explicit local-only escape hatch after remote revocation fails", async () => {
    const subscription = vi
      .spyOn(AccessibilityInfo, "addEventListener")
      .mockReturnValue({ remove: vi.fn() } as unknown as ReturnType<
        typeof AccessibilityInfo.addEventListener
      >);
    const container = testDocument.createElement("div");
    testDocument.body.append(container);
    const root = createRoot(container);
    const forget = vi.fn(async () => {});
    await act(async () => {
      root.render(
        <LoomarrProvider>
          <DeviceDisconnectAction
            density="tv"
            onDisconnect={vi.fn(async () => {
              throw new Error("offline");
            })}
            onForget={forget}
            serverName="https://old.example"
          />
        </LoomarrProvider>,
      );
    });

    await act(async () => buttonNamed(container, "Disconnect device").click());
    await act(async () => buttonNamed(testDocument, "Disconnect").click());

    expect(testDocument.body.textContent).toContain("couldn’t reach the server");
    expect(testDocument.body.textContent).toContain("remain authorized on https://old.example");
    await act(async () => buttonNamed(testDocument, "Forget locally").click());
    expect(forget).toHaveBeenCalledOnce();

    await act(async () => root.unmount());
    container.remove();
    subscription.mockRestore();
  });
});
