import { describe, expect, it, vi } from "vitest";

import { focusTargetProps } from "../src/focus-target";

// The surfaces' "selection outlives focus" behaviour is pinned through the Guide in
// guide-tv-focus.test.tsx; this covers the registry wiring every focus target shares.
describe("focusTargetProps", () => {
  it("registers the handle and reports focus before the control's own handler", () => {
    const calls: string[] = [];
    const registry = {
      focused: vi.fn(() => calls.push("registry")),
      register: vi.fn(),
      request: vi.fn(),
    };
    const props = focusTargetProps(registry, "harbor", () => calls.push("own"));
    const handle = { focus: vi.fn() };

    props.ref(handle);
    props.onFocus();

    expect(registry.register).toHaveBeenCalledWith("harbor", handle);
    expect(registry.focused).toHaveBeenCalledWith("harbor");
    expect(calls).toEqual(["registry", "own"]);
  });

  it("leaves a surface without a registry working", () => {
    const own = vi.fn();
    const props = focusTargetProps(undefined, "harbor", own);
    props.ref(null);
    props.onFocus();
    expect(own).toHaveBeenCalledOnce();
  });
});
