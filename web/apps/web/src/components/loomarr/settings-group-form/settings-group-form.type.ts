import type { SettingEntry, SettingResult } from "@loomarr/api";

interface SettingsGroupFormProps {
  // One registry group's entries (config-design §5) — the wizard renders exactly one
  // group per step, Settings renders one per page. Same component, same PATCH path.
  // These also seed the form: each entry's resolved `value` is its starting value AND
  // the baseline against which "changed" is measured.
  entries: SettingEntry[];
  // Receives ONLY the fields the operator changed — that map is the PATCH `edits` body.
  // (Submitting untouched fields would clear stored secrets; see the component comment.)
  onSave: (edits: Record<string, string>) => void;
  saving?: boolean;
  // Per-key outcomes from the last PATCH.
  results?: SettingResult[];
  // The group's live connection check (config-design §6: configure → validate → save).
  onTest?: () => void;
  testing?: boolean;
  testOk?: boolean;
  testHint?: string;
  className?: string;
}

export type { SettingsGroupFormProps };
