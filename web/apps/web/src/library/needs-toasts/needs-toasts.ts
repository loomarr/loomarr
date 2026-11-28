import { ApiError, toProblem } from "@loomarr/api/mutator";
import { toast } from "sonner";

// What ignoring in Needs you says (#4476 D1). The toast is the quick way back; the Ignored tab is
// the lasting one.

// Long enough to reach by keyboard or screen reader.
const UNDO_MILLIS = 30_000;

// Undo, pressed once: the button stays up while its toast leaves, and a second restore would be
// refused after the first one worked. Its box is the touch target.
const withUndo = (undo: () => Promise<unknown>) => {
  let pressed = false;
  return {
    duration: UNDO_MILLIS,
    actionButtonStyle: { height: 44, minWidth: 44 },
    action: {
      label: "Undo",
      onClick: () => {
        if (pressed) return;
        pressed = true;
        undo().catch((error: unknown) => toastUndoFailed(error, undo));
      },
    },
  };
};

// A refusal says why in the server's own words. Anything else may work on a second try.
const toastUndoFailed = (error: unknown, undo: () => Promise<unknown>) => {
  if (error instanceof ApiError && error.status < 500) {
    const { title, detail } = toProblem(error);
    toast.error(title ?? "Couldn't undo that", { description: detail, duration: UNDO_MILLIS });
    return;
  }
  toast.error("Couldn't undo that", { description: "Try again in a moment.", ...withUndo(undo) });
};

const toastIgnored = (what: string, undo: () => Promise<unknown>) => toast(`Ignored ${what}`, withUndo(undo));

export { toastIgnored, toastUndoFailed };
