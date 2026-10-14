import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import type { SettingResult } from "@loomarr/api/models/settingResult";

interface InstallationLocationProps {
  entries: SettingEntry[];
  values: Record<string, string>;
  onChange: (key: string, value: string) => void;
  results?: SettingResult[];
  error?: string;
  card?: boolean;
  showHeading?: boolean;
}

export type { InstallationLocationProps };
