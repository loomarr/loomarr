import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { FlashOnVolume } from "./flash-on-volume";

describe("FlashOnVolume", () => {
  it("says nothing for the level it mounts with", () => {
    const onChange = vi.fn();
    render(<FlashOnVolume muted={false} onChange={onChange} quiet={false} volume={0.5} />);
    expect(onChange).not.toHaveBeenCalled();
  });

  it("flashes when the level or the mute changes after mount", () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <FlashOnVolume muted={false} onChange={onChange} quiet={false} volume={0.5} />,
    );

    rerender(<FlashOnVolume muted={false} onChange={onChange} quiet={false} volume={0.7} />);
    expect(onChange).toHaveBeenCalledTimes(1);

    rerender(<FlashOnVolume muted onChange={onChange} quiet={false} volume={0.7} />);
    expect(onChange).toHaveBeenCalledTimes(2);
  });

  it("stays away while the session's first play is muting itself", () => {
    const onChange = vi.fn();
    const { rerender } = render(<FlashOnVolume muted={false} onChange={onChange} quiet volume={0.5} />);

    rerender(<FlashOnVolume muted onChange={onChange} quiet volume={0.5} />);

    expect(onChange).not.toHaveBeenCalled();
  });
});
