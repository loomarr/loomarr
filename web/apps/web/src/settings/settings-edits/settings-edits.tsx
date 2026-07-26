import type { ReactNode } from "react";
import { createContext, useCallback, useContext, useMemo, useState } from "react";

// The Settings edit buffer, owned by the LAYOUT rather than a page (config-design §5, V9).
//
// ⚠ This is the whole point of the cross-tab save bar. `SettingsPage` used to hold `edits` in
// its own `useState`, so switching tabs unmounted the page and silently discarded them — an
// operator who edited a connection, stepped over to check a default, and came back found their
// first edit gone with nothing having said so. Hoisting the buffer above the `<Outlet />` makes
// the tab bar navigation rather than a commit boundary.
//
// One map keyed by setting id, one bar, one Save. Per-tab buffers were the alternative and are
// worse: the bar's "3 unsaved" would then mean something different depending on where you were
// standing, which is more confusing than no count at all.

type SettingsEditsValue = {
  edits: Record<string, string>;
  setEdit: (key: string, value: string) => void;
  // Replaces the whole buffer. Used by Discard (clear) and after a successful Save, where the
  // saved values become the new baseline.
  resetEdits: () => void;
};

const SettingsEditsContext = createContext<SettingsEditsValue | undefined>(undefined);

const SettingsEditsProvider = ({ children }: { children: ReactNode }) => {
  const [edits, setEdits] = useState<Record<string, string>>({});

  const setEdit = useCallback((key: string, value: string) => {
    setEdits((prev) => ({ ...prev, [key]: value }));
  }, []);

  const resetEdits = useCallback(() => setEdits({}), []);

  const value = useMemo(() => ({ edits, setEdit, resetEdits }), [edits, setEdit, resetEdits]);
  return <SettingsEditsContext.Provider value={value}>{children}</SettingsEditsContext.Provider>;
};

// useSettingsEdits reads the shared buffer.
//
// Throws outside a provider rather than falling back to local state: a silent fallback would
// restore exactly the bug this exists to fix — the page would work, look right, and lose edits
// on tab switch, which is the failure mode that is hardest to notice.
const useSettingsEdits = (): SettingsEditsValue => {
  const ctx = useContext(SettingsEditsContext);
  if (!ctx) {
    throw new Error("useSettingsEdits must be used inside SettingsEditsProvider (the settings layout)");
  }
  return ctx;
};

export { SettingsEditsProvider, useSettingsEdits };
