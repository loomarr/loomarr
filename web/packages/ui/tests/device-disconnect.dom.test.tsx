// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import { AccessibilityInfo } from "react-native";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DeviceDisconnectAction } from "../pairing";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const buttonNamed = (container: ParentNode, name: string) => {
  const button = Array.from(container.querySelectorAll<HTMLElement>('[role="button"], button')).find(
    (candidate) => candidate.textContent?.trim() === name,
  );
  if (!button) throw new Error(`missing button: ${name}`);
  return button;
};

describe("DeviceDisconnectAction", () => {
  it("offers an explicit local-only escape hatch after remote revocation fails", async () => {
    vi.spyOn(AccessibilityInfo, "addEventListener").mockReturnValue({
      remove: vi.fn(),
    } as unknown as ReturnType<typeof AccessibilityInfo.addEventListener>);
    const forget = vi.fn(async () => {});
    const { container } = renderUi(
      <DeviceDisconnectAction
        density="tv"
        onDisconnect={vi.fn(async () => {
          throw new Error("offline");
        })}
        onForget={forget}
        serverName="https://old.example"
      />,
    );

    await act(async () => buttonNamed(container, "Disconnect device").click());
    await act(async () => buttonNamed(document, "Disconnect").click());

    expect(document.body.textContent).toContain("couldn’t reach the server");
    expect(document.body.textContent).toContain("remain authorized on https://old.example");
    await act(async () => buttonNamed(document, "Forget locally").click());
    expect(forget).toHaveBeenCalledOnce();
  });
});
