import type { Density } from "../tokens";

type ToggleProps = {
  /**
   * Ids of the help, unavailable and validation lines the control is read with, for a toggle whose
   * explanation lives outside it (a settings row's FieldHints).
   */
  "aria-describedby"?: string;
  checked: boolean;
  density?: Density;
  description?: string;
  disabled?: boolean;
  /** The control's own id, for a row label or test that addresses it. */
  id?: string;
  kind?: "checkbox" | "switch";
  label: string;
  /**
   * Announce the label without printing it, for a control beside content that already shows the
   * same words (a row's select box next to the row's own title). The accessible name stays `label`.
   */
  labelHidden?: boolean;
  onCheckedChange: (checked: boolean) => void;
};

export type { ToggleProps };
