import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

const haptics = vi.hoisted(() => ({
  NotificationFeedbackType: { Success: "success", Warning: "warning" },
  notificationAsync: vi.fn(() => Promise.resolve()),
  selectionAsync: vi.fn(() => Promise.resolve()),
}));
const platform = vi.hoisted(() => ({ OS: "ios", isTV: false }));

vi.mock("expo-haptics", () => haptics);
vi.mock("react-native", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-native")>()),
  Platform: platform,
}));

import { type HapticFeedback, useHaptic } from "@loomarr/design-system/haptics";

// Renders the hook once and hands back what it returned.
const haptic = (): HapticFeedback => {
  let captured: HapticFeedback | undefined;
  const Probe = () => {
    captured = useHaptic();
    return null;
  };
  renderToStaticMarkup(<Probe />);
  if (!captured) throw new Error("useHaptic did not render");
  return captured;
};

const tapAll = () => {
  const tap = haptic();
  tap.selection();
  tap.success();
  tap.warning();
};

describe("useHaptic", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    platform.OS = "ios";
    platform.isTV = false;
  });

  it("plays the selection tick and the success and warning notifications on a phone", () => {
    tapAll();
    expect(haptics.selectionAsync).toHaveBeenCalledTimes(1);
    expect(haptics.notificationAsync).toHaveBeenCalledTimes(2);
    expect(haptics.notificationAsync).toHaveBeenNthCalledWith(1, "success");
    expect(haptics.notificationAsync).toHaveBeenNthCalledWith(2, "warning");
  });

  it("is a no-op on the web", () => {
    platform.OS = "web";
    tapAll();
    expect(haptics.selectionAsync).not.toHaveBeenCalled();
    expect(haptics.notificationAsync).not.toHaveBeenCalled();
  });

  it("is a no-op on a TV", () => {
    platform.isTV = true;
    tapAll();
    expect(haptics.selectionAsync).not.toHaveBeenCalled();
    expect(haptics.notificationAsync).not.toHaveBeenCalled();
  });

  it("never breaks the press when the device refuses a haptic", async () => {
    haptics.selectionAsync.mockRejectedValueOnce(new Error("no taptic engine"));
    const tap = haptic();
    expect(() => tap.selection()).not.toThrow();
    // The rejection is swallowed on the next microtask; an unhandled one would fail the run.
    await Promise.resolve();
  });
});
