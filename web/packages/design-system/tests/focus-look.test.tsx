// @vitest-environment jsdom
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { ModalLayer, useFocusLook } from "../index";

describe("useFocusLook", () => {
  it("tracks focus and still calls the control's own handlers", () => {
    const onFocus = vi.fn();
    const onBlur = vi.fn();
    const { result } = renderHook(() => useFocusLook<string>({ onBlur, onFocus }));
    expect(result.current.focused).toBe(false);

    act(() => result.current.onFocus("focus"));
    expect(result.current.focused).toBe(true);
    expect(onFocus).toHaveBeenCalledWith("focus");

    act(() => result.current.onBlur("blur"));
    expect(result.current.focused).toBe(false);
    expect(onBlur).toHaveBeenCalledWith("blur");
  });

  it("drops the look while a modal is open over the control", () => {
    let open = false;
    const wrapper = ({ children }: { children: ReactNode }) => (
      <>
        {children}
        <ModalLayer visible={open}>{null}</ModalLayer>
      </>
    );
    const { result, rerender } = renderHook(() => useFocusLook(), { wrapper });
    act(() => result.current.onFocus(undefined));
    expect(result.current.focused).toBe(true);

    open = true;
    rerender();
    expect(result.current.focused).toBe(false);

    open = false;
    rerender();
    expect(result.current.focused).toBe(true);
  });
});
