import type { SettingEntry, SettingResult } from "@loomarr/api";

interface SettingsGroupFormProps {
  // One registry group's entries (config-design §5) — the wizard renders exactly one
  // group per step, Settings renders one per page. Same component, same PATCH path.
  entries: SettingEntry[];
  // key → edited value. Every value is a string (the PATCH `edits` map shape).
  values: Record<string, string>;
  onChange: (key: string, value: string) => void;
  onSave?: () => void;
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
