import { Field, Surface } from "@loomarr/design-system";

import type { YearFieldProps } from "./year-field.type";

// YearField — a four-digit year in a design-system Field, narrow enough that a From/To pair and its
// Remove button share a row (filler criteria, channel date policies). The text stays the caller's:
// a half-typed "19" is a draft, and the caller decides when a pair is a range.
const YearField = ({ onBlur, ...field }: YearFieldProps) => (
  <Surface backgroundColor="transparent" borderWidth={0} width={112}>
    <Field {...field} keyboardType="number-pad" maxLength={4} onBlur={() => onBlur?.()} />
  </Surface>
);

export { YearField };
