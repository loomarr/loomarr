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
  // Compact renders ONLY the control — no label, doc tooltip, provenance badge or audit line.
  // For the All-settings table (V10), whose own columns already carry the key, group and
  // provenance; repeating them inside the cell would be the same facts twice.
  //
  // A mode rather than a second component on purpose: the control logic is where the real
  // decisions live (which input a `kind` maps to, env-pinned disabling, the secret
  // replace-flow), and a table that reimplemented those would drift from the editors on
  // exactly the details that matter.
  compact?: boolean;
  // Id of the element that visibly labels this control, for `compact` mode — which renders no
  // <label> of its own. In the all-settings table the visible label IS the Key cell, so the
  // control points at it rather than growing a duplicate. Without this the input is labelled
  // only by `title`, which axe rates a SERIOUS violation (`label-title-only`): a sighted mouse
  // user gets a tooltip, a screen-reader user may get nothing.
  labelledBy?: string;
  className?: string;
}

export type { SettingFieldProps };
