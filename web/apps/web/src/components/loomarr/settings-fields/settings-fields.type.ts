import type { SettingEntry, SettingResult } from "@loomarr/api";

interface SettingsFieldsProps {
  // One registry group's entries (config-design §5).
  entries: SettingEntry[];
  // Controlled: the OWNER holds the edit state. That is the whole point of this split —
  // the wizard saves one group per step, while a Settings page saves several blocks from
  // a single sticky bar (§5), so neither can own the form on the group's behalf.
  values: Record<string, string>;
  onChange: (key: string, value: string) => void;
  results?: SettingResult[];
  className?: string;
}

export type { SettingsFieldsProps };
