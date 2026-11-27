import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { HoldWhile } from "./hold-while";

describe("HoldWhile", () => {
  it("holds the controls once while on, and lets go once when it goes off", () => {
    const hold = vi.fn();
    const { rerender } = render(<HoldWhile hold={hold} on />);
    expect(hold.mock.calls).toEqual([[true]]);

    rerender(<HoldWhile hold={hold} on={false} />);
    expect(hold.mock.calls).toEqual([[true], [false]]);
  });

  it("never holds while off, and lets go when it unmounts while on", () => {
    const hold = vi.fn();
    const off = render(<HoldWhile hold={hold} on={false} />);
    off.unmount();
    expect(hold).not.toHaveBeenCalled();

    const on = render(<HoldWhile hold={hold} on />);
    on.unmount();
    expect(hold.mock.calls).toEqual([[true], [false]]);
  });
});
