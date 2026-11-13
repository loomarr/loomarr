import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { HideForIdent } from "./hide-for-ident";

describe("HideForIdent", () => {
  it("hides the controls when the station-ID card arrives, and not before", () => {
    const hide = vi.fn();
    const { rerender } = render(<HideForIdent hide={hide} ident={false} />);
    expect(hide).not.toHaveBeenCalled();

    rerender(<HideForIdent hide={hide} ident />);
    expect(hide).toHaveBeenCalledTimes(1);
  });
});
