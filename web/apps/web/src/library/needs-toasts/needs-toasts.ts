import { ApiError, toProblem } from "@loomarr/api/mutator";
import { toast } from "sonner";

// What ignoring or keeping in Needs you says (#4476 D1, D2). The toast is the quick way back; for an
// ignored title the Ignored tab is the lasting one.

// Long enough to reach by keyboard or screen reader.
const UNDO_MILLIS = 30_000;

// Undo, pressed once: the button stays up while its toast leaves, and a second restore would be
// refused after the first one worked. Its box is the touch target.
// What a failure is called, and what its button says, for each way of asking.
const UNDO = { label: "Undo", failed: "Couldn't undo that" };
const BRING_BACK = { label: "Try again", failed: "Couldn't bring it back" };

const withAction = (words: typeof UNDO, run: () => Promise<unknown>) => {
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

const withUndo = (undo: () => Promise<unknown>) => withAction(UNDO, undo);

// A refusal says why in the server's own words. Anything else may work on a second try.
const toastFailed = (words: typeof UNDO, error: unknown, run: () => Promise<unknown>) => {
  if (error instanceof ApiError && error.status < 500) {
    const { title, detail } = toProblem(error);
    toast.error(title ?? words.failed, { description: detail, duration: UNDO_MILLIS });
    return;
  }
  toast.error(words.failed, { description: "Try again in a moment.", ...withAction(words, run) });
};

const toastBringBackFailed = (error: unknown, bring: () => Promise<unknown>) =>
  toastFailed(BRING_BACK, error, bring);

const toastIgnored = (what: string, undo: () => Promise<unknown>) => toast(`Ignored ${what}`, withUndo(undo));

const toastKept = (what: string, undo: () => Promise<unknown>) => toast(`Kept ${what}`, withUndo(undo));

export { toastBringBackFailed, toastIgnored, toastKept };
