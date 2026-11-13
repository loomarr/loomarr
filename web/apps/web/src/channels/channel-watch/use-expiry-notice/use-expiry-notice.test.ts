import { renderHook } from "@testing-library/react";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useExpiryNotice } from "./use-expiry-notice";

vi.mock("sonner", () => ({ toast: { info: vi.fn() } }));

const NOTICE = "That paused point is no longer available, so you're back live.";

beforeEach(() => vi.mocked(toast.info).mockClear());

describe("useExpiryNotice", () => {
  it("says nothing until the transport raises a notice", () => {
    renderHook(() => useExpiryNotice("a", 0));
    expect(toast.info).not.toHaveBeenCalled();
  });

  it("says it once per new notice, not on every render", () => {
    const { rerender } = renderHook(({ revision }) => useExpiryNotice("a", revision), {
      initialProps: { revision: 1 },
    });
    expect(toast.info).toHaveBeenCalledWith(NOTICE);

    rerender({ revision: 1 });
    expect(toast.info).toHaveBeenCalledTimes(1);

    rerender({ revision: 2 });
    expect(toast.info).toHaveBeenCalledTimes(2);
  });

  it("starts the count again for another channel", () => {
    const { rerender } = renderHook(({ channelId, revision }) => useExpiryNotice(channelId, revision), {
      initialProps: { channelId: "a", revision: 3 },
    });
    expect(toast.info).toHaveBeenCalledTimes(1);

    rerender({ channelId: "b", revision: 1 });

    expect(toast.info).toHaveBeenCalledTimes(2);
  });
});
