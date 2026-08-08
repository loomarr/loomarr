import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { HoldControlsContext, useHoldControls } from "./hold-controls-context";

// The hold context is what lets a control inside the player's bar keep the overlay from auto-hiding
// while it interacts (an open menu). Two guarantees are load-bearing and worth pinning: the DEFAULT
// is a safe no-op (a control used outside a player does nothing, never throws), and a provider's
// value reaches a consumer unchanged (that is the whole wiring).
describe("hold-controls-context", () => {
  it("defaults to a no-op hold — safe outside a player", () => {
    const { result } = renderHook(() => useHoldControls());
    // The contract is "does nothing and does not throw", not a particular return value.
    expect(() => result.current.hold(true)).not.toThrow();
    expect(() => result.current.hold(false)).not.toThrow();
  });

  it("delivers the provider's hold to a consumer", () => {
    const calls: boolean[] = [];
    const value = { hold: (on: boolean) => calls.push(on) };
    const { result } = renderHook(() => useHoldControls(), {
      wrapper: ({ children }) => (
        <HoldControlsContext.Provider value={value}>{children}</HoldControlsContext.Provider>
      ),
    });

    result.current.hold(true);
    result.current.hold(false);
    expect(calls).toEqual([true, false]);
  });
});
