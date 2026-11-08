import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  announceSettingsChange,
  announceSettingsFailure,
  announceSettingsResults,
} from "./settings-feedback";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

describe("settings feedback", () => {
  beforeEach(() => {
    vi.mocked(toast.success).mockReset();
    vi.mocked(toast.error).mockReset();
  });

  it("says one change or several saved", () => {
    announceSettingsResults([{ key: "a", status: "saved" }]);
    expect(toast.success).toHaveBeenLastCalledWith("Change saved");
    announceSettingsResults([
      { key: "a", status: "saved" },
      { key: "b", status: "saved" },
    ]);
    expect(toast.success).toHaveBeenLastCalledWith("Changes saved");
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("flags rejected keys without hiding the ones that saved", () => {
    announceSettingsResults([
      { key: "a", status: "saved" },
      { key: "b", status: "invalid" },
    ]);
    expect(toast.success).toHaveBeenCalledWith("Change saved");
    expect(toast.error).toHaveBeenCalledWith("Some changes need your attention");
  });

  it("confirms an on-the-spot change", () => {
    announceSettingsChange("Cleared");
    expect(toast.success).toHaveBeenCalledWith("Cleared");
  });

  it("says a save failed when no verdict came back", () => {
    announceSettingsFailure(new Error("offline"));
    expect(toast.error).toHaveBeenCalledWith("Couldn't save your changes", { description: "offline" });
  });
});
