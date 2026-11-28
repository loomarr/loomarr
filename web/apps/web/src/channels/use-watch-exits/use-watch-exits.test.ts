import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const router = vi.hoisted(() => ({ back: vi.fn(), canGoBack: true, navigate: vi.fn() }));
vi.mock("@tanstack/react-router", () => ({
  useCanGoBack: () => router.canGoBack,
  useNavigate: () => router.navigate,
  useRouter: () => ({ history: { back: router.back } }),
}));

import { useWatchExits } from "./use-watch-exits";

beforeEach(() => vi.clearAllMocks());

describe("useWatchExits", () => {
  it("goes back where the viewer came from, named Back", () => {
    router.canGoBack = true;
    const { result } = renderHook(() => useWatchExits());
    expect(result.current.back.label).toBe("Back");
    result.current.back.onPress();
    expect(router.back).toHaveBeenCalledOnce();
    expect(router.navigate).not.toHaveBeenCalled();
  });

  it("lands a deep link with no history on the Guide, named for it", () => {
    router.canGoBack = false;
    const { result } = renderHook(() => useWatchExits());
    expect(result.current.back.label).toBe("Guide");
    result.current.back.onPress();
    expect(router.navigate).toHaveBeenCalledWith({ to: "/guide" });
  });

  it("opens the Guide page for the Guide control and G", () => {
    const { result } = renderHook(() => useWatchExits());
    result.current.openGuide();
    expect(router.navigate).toHaveBeenCalledWith({ to: "/guide" });
  });
});
