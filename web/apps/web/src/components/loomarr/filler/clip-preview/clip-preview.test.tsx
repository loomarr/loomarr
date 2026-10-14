import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ClipPreview } from "./clip-preview";

describe("ClipPreview", () => {
  it("uses the exact hash and resets the player when the clip changes", () => {
    const { container, rerender, unmount } = render(
      <ClipPreview clip={{ hash: "first-clip", name: "First commercial" }} />,
    );
    const first = container.querySelector("video");
    expect(first).toHaveAttribute("src", "/v1/filler/media/first-clip");
    expect(first).toHaveAttribute("autoplay");
    rerender(<ClipPreview clip={{ hash: "second-clip", name: "Second commercial" }} />);
    const second = container.querySelector("video");
    expect(second).not.toBe(first);
    expect(second).toHaveAttribute("src", "/v1/filler/media/second-clip");
    unmount();
    expect(container.querySelector("video")).toBeNull();
  });
});
