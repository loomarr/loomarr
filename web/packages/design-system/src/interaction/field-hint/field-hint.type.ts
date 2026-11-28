import type { ReactNode } from "react";

import type { Density } from "../../tokens";

type FieldHintProps = {
  children: ReactNode;
  density?: Density;
  /** The id a control's `aria-describedby` points at, so the hint is read with the field. */
  id?: string;
  /**
   * `alert` announces a validation message the moment it appears. Leave it unset for standing help,
   * which a screen reader reaches through the control's `aria-describedby` instead.
   */
  role?: "alert" | "status";
  tone?: "error" | "help";
};

export type { FieldHintProps };
