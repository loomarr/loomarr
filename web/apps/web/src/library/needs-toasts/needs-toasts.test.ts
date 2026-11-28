import { ApiError } from "@loomarr/api/mutator";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { toastIgnored } from "./needs-toasts";

vi.mock("sonner", () => ({ toast: Object.assign(vi.fn(), { error: vi.fn() }) }));

type Options = { duration: number; action: { onClick: () => void }; actionButtonStyle: object };
const optionsOf = (call: unknown[] | undefined) => call?.[1] as Options;

describe("the Needs you undo toast", () => {
  beforeEach(() => vi.clearAllMocks());

  it("offers Undo for 30 seconds on a 44 px button, and undoes once however often pressed", () => {
    const undo = vi.fn(() => Promise.resolve());
    toastIgnored("a.mkv", undo);
    const options = optionsOf(vi.mocked(toast).mock.calls[0]);
    expect(vi.mocked(toast).mock.calls[0]?.[0]).toBe("Ignored a.mkv");
    expect(options.duration).toBe(30_000);
    expect(options.actionButtonStyle).toEqual({ height: 44, minWidth: 44 });
    options.action.onClick();
    options.action.onClick();
    expect(undo).toHaveBeenCalledTimes(1);
  });

  it("shows the server's problem when the undo is refused", async () => {
    const refused = new ApiError(409, { title: "Already back", detail: "It is waiting again." });
    toastIgnored("a.mkv", () => Promise.reject(refused));
    optionsOf(vi.mocked(toast).mock.calls[0]).action.onClick();
    await vi.waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(toast.error).toHaveBeenCalledWith("Already back", {
      description: "It is waiting again.",
      duration: 30_000,
    });
  });

  it("offers Undo again after a network error", async () => {
    const undo = vi.fn().mockRejectedValueOnce(new TypeError("offline")).mockResolvedValue(undefined);
    toastIgnored("a.mkv", undo);
    optionsOf(vi.mocked(toast).mock.calls[0]).action.onClick();
    await vi.waitFor(() => expect(toast.error).toHaveBeenCalled());
    optionsOf(vi.mocked(toast.error).mock.calls[0]).action.onClick();
    expect(undo).toHaveBeenCalledTimes(2);
  });
});
