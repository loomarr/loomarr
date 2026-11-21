import { Field } from "@loomarr/design-system";
import { createFormHook, createFormHookContexts } from "@tanstack/react-form";
import type { ComponentProps } from "react";

// useUiForm — the TanStack Form every shared form in this package uses, with its field component bound
// once to the design-system Field. A form names a field and says what is particular to it; the label,
// the input's wiring and the validation message are drawn here, so they cannot drift apart between
// forms. It draws no <form> element (native has none): a form submits from its Action, and from the
// field's own `onSubmitEditing`.

const { fieldContext, formContext, useFieldContext } = createFormHookContexts();

type FieldProps = ComponentProps<typeof Field>;

// A validation message: a schema's issue carries `message`, a plain validator returns the string.
const messageOf = (error: unknown): string | undefined => {
  if (typeof error === "string") return error;
  if (
    typeof error === "object" &&
    error !== null &&
    "message" in error &&
    typeof error.message === "string"
  ) {
    return error.message;
  }
  return undefined;
};

// No `onBlur`: TanStack re-validates on blur, and that clears a submit error before the viewer has
// fixed anything. Typing in the field still clears it, which is the right moment.
//
// `error` is a message the form does not own (a refusal that came back from the server); the form's
// own validation message wins over it.
const TextField = ({ error, ...props }: Omit<FieldProps, "invalid" | "onChangeText" | "value">) => {
  const field = useFieldContext<string>();
  const message = field.state.meta.errors.map(messageOf).find((issue) => issue !== undefined);
  return (
    <Field {...props} error={message ?? error} onChangeText={field.handleChange} value={field.state.value} />
  );
};

const { useAppForm: useUiForm } = createFormHook({
  fieldComponents: { TextField },
  fieldContext,
  formComponents: {},
  formContext,
});

export { useUiForm };
