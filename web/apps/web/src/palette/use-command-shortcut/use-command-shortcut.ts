import { useEffect } from "react";

// useCommandShortcut binds ⌘K / Ctrl+K to the palette's open state.
//
// Separate from CommandPalette because the palette's popup only exists while open, and a
// listener living inside it could never re-open it. The shortcut has to outlive the thing
// it opens.
//
// ⚠ **ESCAPE IS NOT BOUND HERE ANY MORE (V50b), and the reason is worth keeping.** It used to be,
// because the palette was a hand-rolled overlay with no dismiss of its own and `SearchCommand`
// deliberately leaves Escape to its consumer (that file explains why: the key must not be handled
// twice). The palette is a real Dialog now and the primitive owns Escape — including restoring
// focus to whatever opened it, which a window-level listener never did. Binding it here as well
// would be a second closer racing the first.
const useCommandShortcut = (setOpen: (fn: (open: boolean) => boolean) => void): void => {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // ⌘K on macOS, Ctrl+K elsewhere — the convention users already have.
      if (e.key.toLowerCase() === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((open) => !open);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [setOpen]);
};

export { useCommandShortcut };
