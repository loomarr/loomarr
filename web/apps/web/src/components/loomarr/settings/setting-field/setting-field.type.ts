import type { SettingEntry, SettingResult } from "@loomarr/api";

interface SettingFieldProps {
  // The generated registry entry (§8) — kind/enum/secret/provenance/doc all come from
  // the BE; nothing about a setting is hand-mirrored on the FE.
  entry: SettingEntry;
  // Every edit is a string: PATCH /v1/settings takes `edits: {[key]: string}`.
  value: string;
  onChange: (value: string) => void;
  // Per-key outcome of the last save: invalid(problem) | pinned | saved.
  result?: SettingResult;
  className?: string;
}

export type { SettingFieldProps };
