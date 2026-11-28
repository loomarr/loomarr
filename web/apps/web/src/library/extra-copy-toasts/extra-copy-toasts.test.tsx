import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppToaster } from "@/components/loomarr/feedback/app-toaster";
import { render } from "@/test/render";
import { toastRemoved, toastRestoreFailed, toastRestoreRefused } from "./extra-copy-toasts";

// jsdom has no pointer capture, which a toast asks for when its button is pressed.
Element.prototype.setPointerCapture ??= () => {};

const show = (say: () => void) => {
  render(<AppToaster />);
  act(say);
};

describe("extra copy toasts", () => {
  afterEach(() => {
    toast.dismiss();
    vi.useRealTimers();
  });

  it("offers Undo on a removal, and runs it once", async () => {
    const undo = vi.fn();
    show(() => toastRemoved("Echo DVD.mkv", undo));
    expect(await screen.findByText("Moved Echo DVD.mkv to the trash")).toBeInTheDocument();
    await userEvent.dblClick(screen.getByRole("button", { name: "Undo" }));
    expect(undo).toHaveBeenCalledTimes(1);
  });

  it("keeps a removal's Undo up for 30 seconds", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    show(() => toastRemoved("Echo DVD.mkv", () => {}));
    await screen.findByRole("button", { name: "Undo" });
    await act(() => vi.advanceTimersByTimeAsync(29_000));
    expect(screen.getByRole("button", { name: "Undo" })).toBeInTheDocument();
    await act(() => vi.advanceTimersByTimeAsync(3_000));
    expect(screen.queryByRole("button", { name: "Undo" })).not.toBeInTheDocument();
  });

  it("shows a refusal's title and detail, with nothing to press", async () => {
    show(() => toastRestoreRefused("The file isn't in the trash", "Check that its drive is connected."));
    expect(await screen.findByText("The file isn't in the trash")).toBeInTheDocument();
    expect(screen.getByText("Check that its drive is connected.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Undo" })).not.toBeInTheDocument();
  });

  it("offers Undo again after a failure", async () => {
    const retry = vi.fn();
    show(() => toastRestoreFailed("Echo DVD.mkv", retry));
    expect(await screen.findByText("Couldn't put Echo DVD.mkv back")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(retry).toHaveBeenCalledTimes(1);
  });
});
