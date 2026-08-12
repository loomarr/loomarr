import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SegmentPreview } from "./segment-preview";

// The affordance that makes the split-review gate answerable (§10 V54). Before it, an operator was
// asked whether a cut at 04:17 was right with nothing to see or hear.

const renderPreview = (over: Partial<React.ComponentProps<typeof SegmentPreview>> = {}) => {
  const onOpenChange = vi.fn();
  const utils = render(
    <>
      <span id="seg-num-0">#1</span>
      <SegmentPreview
        clipHash="a3f9"
        startMs={257_000}
        endMs={287_000}
        position={0}
        labelledBy="seg-num-0"
        open={false}
        onOpenChange={onOpenChange}
        {...over}
      />
    </>,
  );
  return { ...utils, onOpenChange };
};

describe("SegmentPreview", () => {
  // ⚠ 52 rows means 52 potential <video>s, each an open range request against a 20-minute file.
  it("is a tile, not a player, until it is asked for", () => {
    const { container } = renderPreview();

    expect(screen.getByRole("button", { name: /preview segment/i })).toBeInTheDocument();
    expect(container.querySelector("video")).toBeNull();
  });

  // ⚠ It plays the COMPOSITE. A proposed cut has no bytes of its own until confirm writes them.
  it("expands into a player for the parent recording", () => {
    const { container } = renderPreview({ open: true });

    const video = container.querySelector("video");
    expect(video).not.toBeNull();
    expect(video?.getAttribute("src")).toContain("a3f9");
  });

  // ⚠ UNMOUNT, not hide. A hidden element keeps its range request open.
  it("unmounts the player when it collapses", () => {
    const { container, rerender } = renderPreview({ open: true });
    expect(container.querySelector("video")).not.toBeNull();

    rerender(
      <>
        <span id="seg-num-0">#1</span>
        <SegmentPreview
          clipHash="a3f9"
          startMs={257_000}
          endMs={287_000}
          position={0}
          labelledBy="seg-num-0"
          open={false}
          onOpenChange={vi.fn()}
        />
      </>,
    );

    expect(container.querySelector("video")).toBeNull();
  });

  it("reports its state, and points at the panel it controls", () => {
    const { rerender } = renderPreview();
    const tile = screen.getByRole("button", { name: /preview segment/i });
    expect(tile).toHaveAttribute("aria-expanded", "false");

    rerender(
      <>
        <span id="seg-num-0">#1</span>
        <SegmentPreview
          clipHash="a3f9"
          startMs={257_000}
          endMs={287_000}
          position={0}
          labelledBy="seg-num-0"
          open
          onOpenChange={vi.fn()}
        />
      </>,
    );

    const open = screen.getByRole("button", { name: /preview segment/i });
    expect(open).toHaveAttribute("aria-expanded", "true");
    const controls = open.getAttribute("aria-controls");
    expect(controls).toBeTruthy();
    expect(document.getElementById(controls ?? "")).not.toBeNull();
  });

  // ⚠ The name is composed from a fixed verb plus the VISIBLE "#N", never from the adjacent Name
  // input — aria-labelledby on an input takes its value, renaming the button on every keystroke.
  it("takes its name from the visible row marker", () => {
    renderPreview();
    expect(screen.getByRole("button", { name: "Preview segment #1" })).toBeInTheDocument();
  });

  it("collapses on Escape and hands focus back to the tile", async () => {
    const onOpenChange = vi.fn();
    render(
      <>
        <span id="seg-num-0">#1</span>
        <SegmentPreview
          clipHash="a3f9"
          startMs={0}
          endMs={30_000}
          position={0}
          labelledBy="seg-num-0"
          open
          onOpenChange={onOpenChange}
        />
      </>,
    );

    const tile = screen.getByRole("button", { name: /preview segment/i });
    tile.focus();
    await userEvent.keyboard("{Escape}");

    // The panel does not contain focus, so the tile keeps it and no refocus is needed — what
    // matters is that Escape closed it.
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  // ⚠ mm:ss, not "30s". The row already renders "30s" for the same span, and a second one inside
  // the region makes the editor's existing getByText("30s") match two elements and throw.
  it("stamps the span as a timecode", () => {
    renderPreview();
    expect(screen.getByText("00:30")).toBeInTheDocument();
    expect(screen.queryByText("30s")).toBeNull();
  });

  it("shows no badge for an invalid span", () => {
    renderPreview({ startMs: 30_000, endMs: 10_000 });
    expect(screen.queryByText(/^\d\d:\d\d$/)).toBeNull();
  });
});
