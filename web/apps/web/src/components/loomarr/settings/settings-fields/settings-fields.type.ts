import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import type { SettingResult } from "@loomarr/api/models/settingResult";

interface SettingsFieldsProps {
  // One registry group's entries (config-design §5).
  entries: SettingEntry[];
  // Controlled: the OWNER holds the edit state. That is the whole point of this split —
  // the wizard saves one group per step, while a Settings page saves several blocks from
  // a single sticky bar (§5), so neither can own the form on the group's behalf.
  values: Record<string, string>;
  onChange: (key: string, value: string) => void;
  results?: SettingResult[];
  // Take a key back from the environment, or hand it back (config-design §3.1). Passed
  // straight through to each field; omitted on read-only surfaces, where the env lock stays
  // a plain chip with no affordance.
  onEnvOverride?: (key: string, enabled: boolean) => void;
  // Per-key workflow availability. A reason disables that field and is rendered beside it;
  // omitted keys remain ordinary editable settings (config-design §5).
  disabledReasons?: Record<string, string>;
  className?: string;
}

export type { SettingsFieldsProps };
