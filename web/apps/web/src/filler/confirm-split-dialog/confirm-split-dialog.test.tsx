import type { ClipDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConfirmSplitDialog } from "./confirm-split-dialog";

// The confirmation "Split into clips" never had (§10 V54 A8).
//
// ⚠ The assertions are about what the dialog SAYS, not just that it has two buttons. A
// confirmation whose only content is "are you sure?" makes the operator guess at the stakes, and
// guessing wrong in either direction is expensive here: too alarming and 45 parked reels never get
// split, too quiet and someone starts a multi-minute GPU decode by mis-clicking a card in a grid.

const clip = (over: Partial<ClipDTO> = {}): ClipDTO =>
  ({
    hash: "hash-comp",
    name: "80s compilation",
    kind: "commercial",
    durationMs: 900_000,
    isComposite: true,
    tagged: false,
    aiTagged: false,
    playCount: 0,
    playsCounted: true,
    ...over,
  }) as ClipDTO;

describe("ConfirmSplitDialog", () => {
  it("renders nothing without a clip", () => {
    const { container } = render(<ConfirmSplitDialog onConfirm={vi.fn()} onClose={vi.fn()} />);
    expect(container).toBeEmptyDOMElement();
  });

  // ⚠ The real cost is TIME and GPU contention, and it is the only reason to hesitate. An
  // operator who is not told will read a multi-minute silence as a hang.
  it("names the cost, in the clip's own runtime", () => {
    render(<ConfirmSplitDialog clip={clip()} onConfirm={vi.fn()} onClose={vi.fn()} />);

    expect(
      screen.getByRole("heading", { name: /split .*80s compilation.* into clips\?/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/15:00/)).toBeInTheDocument();
    expect(screen.getByText(/several minutes/i)).toBeInTheDocument();
  });

  // ⚠ As load-bearing as the warning. Detection writes a PROPOSAL and nothing else (§10 V34), so
  // an operator who fears losing their compilation is being scared off a safe action.
  it("says plainly that nothing is destroyed or filed yet", () => {
    render(<ConfirmSplitDialog clip={clip()} onConfirm={vi.fn()} onClose={vi.fn()} />);
    expect(screen.getByText(/nothing enters the catalog yet/i)).toBeInTheDocument();
    expect(screen.getByText(/left exactly as it is/i)).toBeInTheDocument();
  });

  it("confirms and cancels through separate handlers", async () => {
    const onConfirm = vi.fn();
    const onClose = vi.fn();
    const { rerender } = render(<ConfirmSplitDialog clip={clip()} onConfirm={onConfirm} onClose={onClose} />);

    await userEvent.click(screen.getByRole("button", { name: /find the clips/i }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();

    rerender(<ConfirmSplitDialog clip={clip()} onConfirm={onConfirm} onClose={onClose} />);
    await userEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onConfirm).toHaveBeenCalledTimes(1); // cancelling starts nothing
  });

  // A missing duration must not render "NaN:NaN" in a sentence about how long this will take.
  it("survives a clip with no measured duration", () => {
    render(
      <ConfirmSplitDialog clip={clip({ durationMs: undefined })} onConfirm={vi.fn()} onClose={vi.fn()} />,
    );
    expect(screen.getByText(/00:00/)).toBeInTheDocument();
  });
});
