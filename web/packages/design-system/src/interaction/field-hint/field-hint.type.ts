import type { ReactNode } from "react";

import type { Density } from "../../tokens";

type FieldHintProps = {
  children: ReactNode;
  /** Web layout hooks only (a grid span, a gap beside an icon). Never colour or type: those are the hint's own. */
  className?: string;
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
