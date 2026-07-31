import { type SettingEntry, settingsApi, unwrap } from "@loomarr/api";

// The settings list is fetched once by the Settings layout and read by each page through
// the shared query cache — one request, not one per page.
const useSettingsEntries = (): SettingEntry[] => {
  const settings = settingsApi.useSettingsList({ query: { retry: false } });
  return unwrap(settings.data, (b) => b.settings) ?? [];
};

export { useSettingsEntries };
