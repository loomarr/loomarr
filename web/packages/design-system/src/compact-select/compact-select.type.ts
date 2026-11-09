import type { Density } from "../tokens";

type CompactSelectOption<Value extends string> = {
  disabled?: boolean;
  label: string;
  value: Value;
};

type CompactSelectProps<Value extends string> = {
  /** Ids of the help and validation lines the select is read with (a settings row's FieldHints). */
  "aria-describedby"?: string;
  /** The id of a row's own visible label, which then names the select in place of `label`. */
  "aria-labelledby"?: string;
  density?: Density;
  disabled?: boolean;
  /** The select's own id, for a caller's <label htmlFor> or a test that addresses it. */
  id?: string;
  invalid?: boolean;
  /** The accessible name. Never printed: the select sits where its place already says what it picks. */
  label: string;
  onValueChange: (value: Value) => void;
  options: readonly CompactSelectOption<Value>[];
  /**
   * A value something else owns (a setting the environment pins): it cannot be changed, but it is
   * real, so it reads at full strength rather than in the dimmed disabled look of an empty field.
   */
  readOnly?: boolean;
  value: Value;
  /** A fixed width for a toolbar slot; without one the select fills its row. */
  width?: number;
};

export type { CompactSelectOption, CompactSelectProps };
