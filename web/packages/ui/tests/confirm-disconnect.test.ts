import { beforeEach, describe, expect, it, vi } from "vitest";

const { alert, platform, showActionSheet } = vi.hoisted(() => ({
  alert: vi.fn(),
  platform: { OS: "ios" },
  showActionSheet: vi.fn(),
}));
// Only the three platform seams are faked; the rest of react-native stays real so the package entrypoint loads.
vi.mock("react-native", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-native")>()),
  ActionSheetIOS: { showActionSheetWithOptions: showActionSheet },
  Alert: { alert },
  Platform: platform,
}));

const { confirmDisconnect } = await import("../index");

beforeEach(() => {
  alert.mockReset();
  showActionSheet.mockReset();
  platform.OS = "ios";
});

describe("confirmDisconnect", () => {
  it("asks on iOS with a destructive Disconnect and a Cancel", () => {
    const confirm = vi.fn();
    confirmDisconnect("tv.example.com", confirm);

    const [options, choose] = showActionSheet.mock.calls[0] ?? [];
    expect(options).toMatchObject({
      cancelButtonIndex: 1,
      destructiveButtonIndex: 0,
      options: ["Disconnect", "Cancel"],
    });
    choose(1);
    expect(confirm).not.toHaveBeenCalled();
    choose(0);
    expect(confirm).toHaveBeenCalledTimes(1);
  });

  it("asks with an alert elsewhere, Disconnect marked destructive", () => {
    platform.OS = "android";
    const confirm = vi.fn();
    confirmDisconnect("tv.example.com", confirm);

    const buttons = alert.mock.calls[0]?.[2] as { onPress?: () => void; style: string; text: string }[];
    expect(buttons.map(({ style, text }) => [text, style])).toEqual([
      ["Cancel", "cancel"],
      ["Disconnect", "destructive"],
    ]);
    buttons[1]?.onPress?.();
    expect(confirm).toHaveBeenCalledTimes(1);
  });
});
