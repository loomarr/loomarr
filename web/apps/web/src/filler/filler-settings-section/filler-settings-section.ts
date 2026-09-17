const SETTINGS_SECTIONS = [
  { id: "downloads", label: "Automatic downloads" },
  { id: "folders", label: "Clip folders" },
  { id: "storage", label: "Storage limits" },
  { id: "incoming", label: "Incoming history" },
  { id: "breaks", label: "Commercial breaks" },
  { id: "review", label: "Clip review" },
  { id: "playback", label: "Clip eligibility and sound" },
  { id: "limits", label: "Processing limits" },
  { id: "tools", label: "Processing tools" },
] as const;

type FillerSettingsSection = (typeof SETTINGS_SECTIONS)[number]["id"];

const parseFillerSettingsSection = (value: unknown): FillerSettingsSection | undefined =>
  SETTINGS_SECTIONS.find((item) => item.id === value)?.id;

export type { FillerSettingsSection };
export { parseFillerSettingsSection, SETTINGS_SECTIONS };
