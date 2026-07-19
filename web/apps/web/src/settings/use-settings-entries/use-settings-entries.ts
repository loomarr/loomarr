import { type SettingEntry, settingsApi } from "@loomarr/api";

// The settings list is fetched once by the Settings layout and read by each page through
// the shared query cache — one request, not one per page.
const useSettingsEntries = (): SettingEntry[] => {
  const settings = settingsApi.useSettingsList({ query: { retry: false } });
  return settings.data?.status === 200 ? (settings.data.data.settings ?? []) : [];
};

export { useSettingsEntries };
