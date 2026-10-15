// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { AccessibilityInfo } from "react-native";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DeviceSheet, type DeviceSheetProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type Node = { click(): void; getAttribute(name: string): string | null; textContent?: string | null };
type Container = Parameters<typeof createRoot>[0] & {
  querySelectorAll(selector: string): ArrayLike<Node>;
  remove(): void;
  textContent?: string | null;
};
const testDocument = (
  globalThis as unknown as {
    document: { body: { append(node: unknown): void }; createElement(tag: string): Container };
  }
).document;

const noop = vi.fn();
const props: DeviceSheetProps = {
  account: { name: "Sam", role: "admin" },
  appVersion: "1.4.0 (212)",
  onDisconnect: noop,
  onDismiss: noop,
  serverUrl: "https://tv.example.com:8443/some/path?x=1",
  serverVersion: "v0.9.2",
};

let container: Container;
let root: ReturnType<typeof createRoot>;
beforeEach(() => {
  vi.spyOn(AccessibilityInfo, "addEventListener").mockReturnValue({
    remove: vi.fn(),
  } as unknown as ReturnType<typeof AccessibilityInfo.addEventListener>);
  container = testDocument.createElement("div");
  testDocument.body.append(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.restoreAllMocks();
  noop.mockReset();
});

const show = async (over: Partial<DeviceSheetProps> = {}) => {
  await act(async () => {
    root.render(
      <LoomarrProvider>
        <DeviceSheet {...props} {...over} />
      </LoomarrProvider>,
    );
  });
};
const labels = () =>
  Array.from(container.querySelectorAll("[aria-label]")).map((node) => node.getAttribute("aria-label"));
const disconnectButton = () => {
  const button = Array.from(container.querySelectorAll('[role="button"]')).find((node) =>
    node.textContent?.includes("isconnect"),
  );
  if (!button) throw new Error("missing the Disconnect button");
  return button;
};

describe("DeviceSheet", () => {
  it("reads who, where and which build, in order, with the server as host and port only", async () => {
    await show();
    expect(labels()).toEqual([
      "Device and account",
      "Signed in as, Sam, Admin",
      "Server, tv.example.com:8443",
      "Server version, v0.9.2",
      "App version, 1.4.0 (212)",
    ]);
    expect(container.textContent).not.toContain("https://");
  });

  it("leaves out what it doesn't know rather than guessing", async () => {
    await show({ account: undefined, appVersion: undefined, serverVersion: undefined });
    expect(labels()).toEqual(["Device and account", "Server, tv.example.com:8443"]);
  });

  it("disconnects only after the confirmation says so", async () => {
    const confirmDisconnect = vi.fn();
    await show({ confirmDisconnect });
    await act(async () => disconnectButton().click());
    expect(confirmDisconnect).toHaveBeenCalledWith("tv.example.com:8443", expect.any(Function));
    expect(noop).not.toHaveBeenCalled();

    await act(async () => confirmDisconnect.mock.calls[0]?.[1]());
    expect(noop).toHaveBeenCalledTimes(1);
  });

  it("stays open with a retry when the server can't be reached", async () => {
    const onDisconnect = vi.fn().mockRejectedValue(new Error("offline"));
    const confirmDisconnect = vi.fn((_address: string, confirm: () => void) => confirm());
    await show({ confirmDisconnect, onDisconnect });
    await act(async () => disconnectButton().click());
    expect(container.textContent).toContain("couldn't reach tv.example.com:8443");
    expect(disconnectButton().textContent).toBe("Try disconnecting again");
  });
});
