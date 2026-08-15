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
  // Take this key back from the environment, or hand it back (config-design §3.1).
  //
  // Optional, and its ABSENCE is what keeps the pre-3.1 behaviour available: a surface that
  // does not supply it renders the plain "set via environment" lock with no way out, which is
  // still correct for read-only contexts. Supplying it turns the lock into an affordance.
  //
  // Deliberately NOT folded into `onChange`: unlocking is a separate durable act that commits
  // immediately (it is one of §9's inline-commit verbs), while `onChange` stages an edit into
  // the cross-tab save buffer. Routing both through one handler would either make the unlock
  // wait for a Save the operator cannot reach, or make ordinary edits commit instantly.
  onEnvOverride?: (enabled: boolean) => void;
  // A workflow-level prerequisite that prevents this value from taking effect. Distinct from
  // `provenance: env`: this disables the control because the capability cannot run, and renders
  // the reason beside it while retaining the desired value (config-design §5).
  disabledReason?: string;
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
