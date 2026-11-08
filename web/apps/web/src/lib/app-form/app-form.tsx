import { Field } from "@loomarr/design-system";
import { createFormHook, createFormHookContexts } from "@tanstack/react-form";
import type { ComponentProps } from "react";
import { FieldError } from "@/components/loomarr/feedback/field-error";
import { cn } from "@/lib/utils";

// useAppForm — every web form's TanStack Form, with its field components bound once (§14). A form
// names a field and says what is particular to it; the label, the input's wiring and the validation
// message are drawn here, so they cannot drift apart between forms.

const { fieldContext, formContext, useFieldContext } = createFormHookContexts();

// The browser input types the forms use, mapped onto the design-system Field: a password gets the
// Field's own show-password button, and email and number pick the matching keyboard.
type TextFieldType = "email" | "number" | "password" | "text";

const KEYBOARD: Record<TextFieldType, ComponentProps<typeof Field>["keyboardType"]> = {
  email: "email-address",
  number: "numeric",
  password: "default",
  text: "default",
};

interface TextFieldProps {
  autoComplete?: ComponentProps<typeof Field>["autoComplete"];
  // The field's own wrapper, for its place in the form; the input keeps the Field's look.
  className?: string;
  disabled?: boolean;
  id?: string;
  label: string;
  placeholder?: string;
  type?: TextFieldType;
}

// No `onBlur`: FieldError says why a submit-validated field leaves it out. The message is
// FieldError's alert rather than the Field's own error line, so it is announced on submit.
const TextField = ({
  autoComplete,
  className,
  disabled,
  id,
  label,
  placeholder,
  type = "text",
}: TextFieldProps) => {
  const field = useFieldContext<string>();
  const { errors } = field.state.meta;
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      {/* React Native Web defaults an input to sentence-capitalised and autocorrected; the forms
          take usernames, passwords, emails and short constraints, none of which want either. */}
      <Field
        autoCapitalize="none"
        autoComplete={autoComplete}
        autoCorrect={false}
        disabled={disabled}
        id={id}
        invalid={errors.length > 0}
        keyboardType={KEYBOARD[type]}
        label={label}
        onChangeText={field.handleChange}
        placeholder={placeholder}
        secureTextEntry={type === "password"}
        value={field.state.value}
      />
      <FieldError errors={errors} />
    </div>
  );
};

const { useAppForm } = createFormHook({
  fieldComponents: { TextField },
  fieldContext,
  formComponents: {},
  formContext,
});

export { useAppForm };
