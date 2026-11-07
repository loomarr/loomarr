import { FieldHint } from "@loomarr/design-system";
import type { FieldErrorProps } from "./field-error.type";

// FieldError — a form field's validation message, under the field it is about. Validation runs on
// submit (§14), so the message appears at that moment and is announced as an alert.
//
// ⚠ A submit-validated field leaves out `onBlur={field.handleBlur}`. TanStack re-validates on blur,
// and that clears the submit error: the message vanished the moment someone tabbed or clicked away,
// before they had fixed anything. Typing in the field still clears it, which is the right moment.
const FieldError = ({ errors, className }: FieldErrorProps) => {
  const message = errors.find((error) => error !== undefined)?.message;
  return message ? (
    <FieldHint className={className} role="alert" tone="error">
      {message}
    </FieldHint>
  ) : null;
};

export { FieldError };
