import { toast } from "sonner";

// What removing an extra copy says (#4476 D3). Its toast is the one place the removal can be
// undone; after it, the file waits in the trash folder.

// Long enough to reach by keyboard or screen reader.
const UNDO_MILLIS = 30_000;

// Undo, pressed once: the button stays up while its toast leaves, and a second restore would be
// refused after the first one worked. Its box is the touch target.
const withUndo = (undo: (() => void) | undefined) => {
  let pressed = false;
  return {
    duration: UNDO_MILLIS,
    actionButtonStyle: { height: 44, minWidth: 44 },
    action: undo
      ? {
          label: "Undo",
          onClick: () => {
            if (pressed) return;
            pressed = true;
            undo();
          },
        }
      : undefined,
  };
};

const toastRemoved = (name: string, undo?: () => void) => toast(`Moved ${name} to the trash`, withUndo(undo));

const toastRestored = (name: string) => toast.success(`Put ${name} back`);

// The server won't put it back: its own words say why.
const toastRestoreRefused = (title: string, detail?: string) =>
  toast.error(title, { description: detail, duration: UNDO_MILLIS });

// No answer, or the server failed: the same undo can be tried again.
const toastRestoreFailed = (name: string, retry: () => void) =>
  toast.error(`Couldn't put ${name} back`, {
    description: "Try again in a moment.",
    ...withUndo(retry),
  });

export { toastRemoved, toastRestored, toastRestoreFailed, toastRestoreRefused };
