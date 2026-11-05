import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import type { SettingKey } from "@loomarr/api/models/settingKey";
import type { SettingResult } from "@loomarr/api/models/settingResult";

interface InstallationLocationProps {
  entries: SettingEntry[];
  values: Partial<Record<SettingKey, string>>;
  onChange: (key: SettingKey, value: string) => void;
  results?: SettingResult[];
  error?: string;
  card?: boolean;
  showHeading?: boolean;
}

export type { InstallationLocationProps };
