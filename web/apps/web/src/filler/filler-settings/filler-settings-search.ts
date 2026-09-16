const SETTINGS_SECTIONS = [
  { id: "downloads", label: "Automatic downloads" },
  { id: "folders", label: "Clip folders" },
  { id: "storage", label: "Storage limits" },
  { id: "breaks", label: "Commercial breaks" },
  { id: "review", label: "Clip review" },
  { id: "playback", label: "Clip eligibility and sound" },
  { id: "limits", label: "Processing limits" },
  { id: "tools", label: "Processing tools" },
] as const;

type FillerSettingsSection = (typeof SETTINGS_SECTIONS)[number]["id"];

const validateFillerSettingsSearch = (
  search: Record<string, unknown>,
): { section: FillerSettingsSection } => ({
  section: SETTINGS_SECTIONS.find((item) => item.id === search.section)?.id ?? "downloads",
});

export type { FillerSettingsSection };
export { SETTINGS_SECTIONS, validateFillerSettingsSearch };
