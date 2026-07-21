import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { BrandLockup } from "./brand-lockup";

describe("BrandLockup", () => {
  it("shows the wordmark and tagline in the hero", () => {
    render(<BrandLockup variant="hero" />);
    expect(screen.getByText("LOOMARR")).toBeInTheDocument();
    expect(screen.getByText("always something on")).toBeInTheDocument();
  });

  it("drops the tagline when asked, and in compact", () => {
    const { rerender } = render(<BrandLockup variant="hero" tagline={false} />);
    expect(screen.queryByText("always something on")).not.toBeInTheDocument();
    rerender(<BrandLockup variant="compact" />);
    // Compact is the sidebar mark — wordmark only, no tagline.
    expect(screen.getByText("LOOMARR")).toBeInTheDocument();
    expect(screen.queryByText("always something on")).not.toBeInTheDocument();
  });
});
