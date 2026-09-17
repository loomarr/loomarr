const SETTINGS_SECTIONS = [
  {
    id: "downloads",
    label: "Automatic downloads",
    description: "Choose how often sources are checked and how many clips they add.",
    aliases: ["fetch schedule", "check sources", "download clips"],
    group: "everyday",
  },
  {
    id: "folders",
    label: "Clip folders",
    description: "Choose where clips live and how dropped files enter Loomarr.",
    aliases: ["drop folder", "watch folder", "clip library"],
    group: "everyday",
  },
  {
    id: "storage",
    label: "Storage limits",
    description: "Stop automatic downloads before clips fill this drive.",
    aliases: ["disk full", "stop clips filling disk", "clip limit", "catalog limit", "drive capacity"],
    group: "everyday",
  },
  {
    id: "incoming",
    label: "Incoming history",
    description: "Choose how long ready clips remain visible in Incoming.",
    aliases: ["recent clips", "ready window", "24 hours"],
    group: "everyday",
  },
  {
    id: "breaks",
    label: "Break assembly",
    description: "Choose the usual length and number of clips in a commercial break.",
    aliases: ["commercial length", "clips per break", "ad break"],
    group: "everyday",
  },
  {
    id: "review",
    label: "Clip review",
    description: "Choose which automatic checks can identify and split incoming clips.",
    aliases: ["AI tagging", "vision", "transcribe", "auto split"],
    group: "everyday",
  },
  {
    id: "playback",
    label: "Clip eligibility and sound",
    description: "Choose which clips may play, how often they repeat, and their sound level.",
    aliases: ["language", "loudness", "quality", "repeat cooldown", "duration"],
    group: "everyday",
  },
  {
    id: "limits",
    label: "Processing limits",
    description: "Limit how much clip preparation Loomarr performs in one pass.",
    aliases: ["pipeline", "background processing", "CPU", "GPU", "per pass"],
    group: "advanced",
  },
  {
    id: "tools",
    label: "Processing tools",
    description: "Set executable and model paths for unusual installations.",
    aliases: ["yt-dlp", "FFmpeg", "whisper", "binary paths"],
    group: "advanced",
  },
] as const;

type FillerSettingsSection = (typeof SETTINGS_SECTIONS)[number]["id"];
type FillerSettingsGroup = (typeof SETTINGS_SECTIONS)[number]["group"];

const parseFillerSettingsSection = (value: unknown): FillerSettingsSection | undefined =>
  SETTINGS_SECTIONS.find((item) => item.id === value)?.id;

export type { FillerSettingsGroup, FillerSettingsSection };
export { parseFillerSettingsSection, SETTINGS_SECTIONS };
