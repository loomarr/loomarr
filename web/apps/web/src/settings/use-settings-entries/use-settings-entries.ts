import * as settingsApi from "@loomarr/api/endpoints/settings";
import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import { unwrap } from "@loomarr/api/unwrap";

// The settings list is fetched once by the Settings layout and read by each page through
// the shared query cache — one request, not one per page.
const useSettingsEntries = (): SettingEntry[] => {
  const settings = settingsApi.useSettingsList({ query: { retry: false } });
  return unwrap(settings.data, (b) => b.settings) ?? [];
};

export { useSettingsEntries };
