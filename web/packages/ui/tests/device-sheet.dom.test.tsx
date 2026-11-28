// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import { AccessibilityInfo } from "react-native";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DeviceSheet, type DeviceSheetProps } from "../pairing";

const noop = vi.fn();
const props: DeviceSheetProps = {
  account: { name: "Sam", role: "admin" },
  appVersion: "1.4.0 (212)",
  onDisconnect: noop,
  onDismiss: noop,
  serverUrl: "https://tv.example.com:8443/some/path?x=1",
  serverVersion: "v0.9.2",
};

let container: HTMLElement;
beforeEach(() => {
  vi.spyOn(AccessibilityInfo, "addEventListener").mockReturnValue({
    remove: vi.fn(),
  } as unknown as ReturnType<typeof AccessibilityInfo.addEventListener>);
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  noop.mockReset();
});

const show = async (over: Partial<DeviceSheetProps> = {}) => {
  await act(async () => {
    container = renderUi(<DeviceSheet {...props} {...over} />).container;
  });
};
const labels = () =>
  Array.from(container.querySelectorAll("[aria-label]")).map((node) => node.getAttribute("aria-label"));
const disconnectButton = () => {
  const button = Array.from(container.querySelectorAll<HTMLElement>('[role="button"]')).find((node) =>
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
