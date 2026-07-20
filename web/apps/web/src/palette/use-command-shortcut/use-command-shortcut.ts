import { useEffect } from "react";

// useCommandShortcut binds ⌘K / Ctrl+K and Escape to the palette's open state.
//
// Separate from CommandPalette because the palette unmounts when closed (it renders
// null), and a listener living inside it could never re-open it. The shortcut has to
// outlive the thing it opens.
const useCommandShortcut = (setOpen: (fn: (open: boolean) => boolean) => void): void => {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // ⌘K on macOS, Ctrl+K elsewhere — the convention users already have.
      if (e.key.toLowerCase() === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((open) => !open);
        return;
      }
      if (e.key === "Escape") setOpen(() => false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [setOpen]);
};

export { useCommandShortcut };
