import type { Density } from "../tokens";

type DateFieldProps = {
  density?: Density;
  /** Standing help under the box, as a Field prints it. */
  description?: string;
  disabled?: boolean;
  /** The input's own id, for a test or a caller's own wiring. */
  id?: string;
  /** `date` takes YYYY-MM-DD; `datetime` takes YYYY-MM-DDTHH:MM in local time, as the browser does. */
  kind?: "date" | "datetime";
  label: string;
  onValueChange: (value: string) => void;
  /** The browser's value: "" when cleared or unset. */
  value: string;
  /** A fixed width for a toolbar slot; without one the field fills its row. */
  width?: number;
};

export type { DateFieldProps };
