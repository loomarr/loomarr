import type { SettingEntry } from "@loomarr/api";

interface AllSettingsTableProps {
  entries: SettingEntry[];
  // Controlled so the page owns the query — it belongs in the URL eventually (a shared link to
  // "every playout key" is useful), and a component holding its own input cannot offer that.
  query: string;
  onQueryChange: (q: string) => void;
  // Staged edits from the cross-tab buffer (V9), and the setter that stages one. Passed in
  // rather than read from the context here so the component stays renderable in the gallery
  // without a provider.
  values: Record<string, string>;
  onEdit: (key: string, value: string) => void;
  className?: string;
}

export type { AllSettingsTableProps };
