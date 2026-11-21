import type { settingsPatchResponse } from "@loomarr/api/endpoints/settings";
import { useCallback, useMemo, useState } from "react";
import { savedKeysOf } from "../saved-edits";

// The draft of a settings save: the keys an operator changed, by setting key, and nothing else.
//
// Settings and the setup wizard both render their fields from the settings registry
// (`SettingsFields`), so a field is a registry row, not a named form field, and `useAppForm` has
// nothing to bind. What they share is this buffer: only the changed keys are sent (a stored secret
// reads back empty, and an empty PATCH would clear it), a saved key stops being a draft, a key the
// server refused stays editable beside its message, and Discard drops the lot.
//
// This is the one sanctioned way to buffer a settings draft: `web/scripts/check-forms.mjs` knows it
// by name (#2147). A step that kept its own `useState` for these edits would fail that check.
type SettingsDraft = {
  edits: Record<string, string>;
  setEdit: (key: string, value: string) => void;
  // Drops these keys from the draft.
  clearEdits: (keys: readonly string[]) => void;
  // Discard: drops every key.
  resetEdits: () => void;
  // After a save: drops the submitted keys the server saved, and keeps the rest for another try.
  dropSaved: (response: settingsPatchResponse, submitted: Record<string, string>) => void;
};

const useSettingsDraft = (): SettingsDraft => {
  const [edits, setEdits] = useState<Record<string, string>>({});
  const setEdit = useCallback((key: string, value: string) => {
    setEdits((previous) => ({ ...previous, [key]: value }));
  }, []);
  const clearEdits = useCallback((keys: readonly string[]) => {
    const cleared = new Set(keys);
    setEdits((previous) => Object.fromEntries(Object.entries(previous).filter(([key]) => !cleared.has(key))));
  }, []);
  const resetEdits = useCallback(() => setEdits({}), []);
  const dropSaved = useCallback(
    (response: settingsPatchResponse, submitted: Record<string, string>) =>
      clearEdits(savedKeysOf(response, submitted)),
    [clearEdits],
  );
  return useMemo(
    () => ({ edits, setEdit, clearEdits, resetEdits, dropSaved }),
    [edits, setEdit, clearEdits, resetEdits, dropSaved],
  );
};

export type { SettingsDraft };
export { useSettingsDraft };
