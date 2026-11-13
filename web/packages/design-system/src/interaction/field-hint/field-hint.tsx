import { isWeb } from "@tamagui/core";

import { Text } from "../../text";
import type { FieldHintProps } from "./field-hint.type";

/**
 * A form field's help or validation line (#970 step 45). On the Web it is a real `<p>`, so a native
 * `<input>`'s `aria-describedby` resolves to it and its text joins the control's accessible
 * description; the control itself stays a browser element, keeping form submission and the
 * browser's own validation. Help reads in the secondary content colour, an error in the danger
 * colour: the two Web conventions it replaces (`text-muted-foreground` / `text-onair-300`).
 *
 * Field's own description and error lines keep their `metadata` measure: the native field has
 * its own approved look, and folding it onto this one would change it.
 */
const FieldHint = ({ children, className, density = "pointer", id, role, tone = "help" }: FieldHintProps) => (
  <Text
    // `role` is outside TextProps on purpose (a readout is not a landmark); a hint is the exception.
    {...({ role } as object)}
    as={isWeb ? "p" : undefined}
    className={className}
    density={density}
    id={id}
    textRole="hint"
    tone={tone === "error" ? "danger" : "secondary"}
  >
    {children}
  </Text>
);

export { FieldHint };
