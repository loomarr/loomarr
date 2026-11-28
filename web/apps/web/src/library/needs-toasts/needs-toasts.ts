import { ApiError, toProblem } from "@loomarr/api/mutator";
import { VisuallyHidden } from "@loomarr/design-system";
import { createElement, Fragment, type ReactNode } from "react";
import { toast } from "sonner";

// What ignoring or keeping in Needs you says (#4476 D1, D2). The toast is the quick way back; for an
// ignored title the Ignored tab is the lasting one.

// Long enough to reach by keyboard or screen reader.
const UNDO_MILLIS = 30_000;

// Undo, pressed once: the button stays up while its toast leaves, and a second restore would be
// refused after the first one worked. Its box is the touch target.
// What a failure is called, and what its button says, for each way of asking.
type Words = { label: ReactNode; failed: string };
const BRING_BACK: Words = { label: "Try again", failed: "Couldn't bring it back" };

// The button shows "Undo" and is named for what it undoes. The toast's button takes no name of its
// own, so the longer words ride inside it, out of sight.
const undoWords = (undoing: string): Words => ({
  label: createElement(
    Fragment,
    null,
    createElement("span", { "aria-hidden": true }, "Undo"),
    createElement(VisuallyHidden, { as: "span" }, `Undo ${undoing}`),
  ),
  failed: "Couldn't undo that",
});

const withAction = (words: Words, run: () => Promise<unknown>) => {
  let pressed = false;
  return {
    duration: UNDO_MILLIS,
    actionButtonStyle: { height: 44, minWidth: 44 },
    action: {
      label: words.label,
      onClick: () => {
        if (pressed) return;
        pressed = true;
        run().catch((error: unknown) => toastFailed(words, error, run));
      },
    },
  };
};

// A refusal says why in the server's own words. Anything else may work on a second try.
const toastFailed = (words: Words, error: unknown, run: () => Promise<unknown>) => {
  if (error instanceof ApiError && error.status < 500) {
    const { title, detail } = toProblem(error);
    toast.error(title ?? words.failed, { description: detail, duration: UNDO_MILLIS });
    return;
  }
  toast.error(words.failed, { description: "Try again in a moment.", ...withAction(words, run) });
};

const toastBringBackFailed = (error: unknown, bring: () => Promise<unknown>) =>
  toastFailed(BRING_BACK, error, bring);

const toastIgnored = (what: string, undo: () => Promise<unknown>) =>
  toast(`Ignored ${what}`, withAction(undoWords(`ignoring ${what}`), undo));

const toastKept = (what: string, undo: () => Promise<unknown>) =>
  toast(`Kept ${what}`, withAction(undoWords(`keeping ${what}`), undo));

export { toastBringBackFailed, toastIgnored, toastKept };
