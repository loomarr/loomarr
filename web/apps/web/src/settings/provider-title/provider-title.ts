import type { SettingEntry } from "@loomarr/api/models/settingEntry";

// A block whose backend is provider-selected (Requester, AI) shows the CHOSEN provider in its
// title — "Requester (Seerr)", "AI (Ollama)" — so at a glance you see what it's wired to. The
// suffix comes from the SAVED value (entry.value), not a live edit, so it only changes after
// Save; an unset/unknown provider yields no suffix ("no suffix until chosen+saved").

// PROVIDER_LABELS maps a provider setting key → its value → the short UI label shown in the
// parenthetical. Concise on purpose (the full enum label reads long in a heading).
const PROVIDER_LABELS: Record<string, Record<string, string>> = {
  "requester.provider": { seerr: "Seerr", arr: "Sonarr + Radarr" },
  "llm.provider": { ollama: "Ollama", openai: "OpenAI-compatible" },
};

// providerSuffix returns " (Label)" for the saved provider value, or "" when the key is
// unset / not a known option. `key` is the provider setting (e.g. "requester.provider").
const providerSuffix = (entries: SettingEntry[], key: string): string => {
  const value = entries.find((e) => e.key === key)?.value ?? "";
  const label = PROVIDER_LABELS[key]?.[value];
  return label ? ` (${label})` : "";
};

// blockTitle composes a base title with its provider suffix — e.g. blockTitle(entries,
// "Requester", "requester.provider") → "Requester (Seerr)".
const blockTitle = (entries: SettingEntry[], base: string, providerKey: string): string =>
  base + providerSuffix(entries, providerKey);

export { blockTitle, PROVIDER_LABELS, providerSuffix };
