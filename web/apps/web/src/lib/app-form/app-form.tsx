import { createFormHook, createFormHookContexts } from "@tanstack/react-form";
import { type ComponentProps, type ReactNode, useId } from "react";
import { FieldError } from "@/components/loomarr/feedback/field-error";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

// useAppForm — every web form's TanStack Form, with its field components bound once (§14). A form
// names a field and says what is particular to it; the label, the input's wiring and the validation
// message are drawn here, so they cannot drift apart between forms.

const { fieldContext, formContext, useFieldContext } = createFormHookContexts();

type TextFieldProps = Omit<ComponentProps<typeof Input>, "aria-invalid" | "name" | "onChange" | "value"> & {
  label: ReactNode;
};

// `className` is the field's own wrapper, for its place in the form; the input keeps its look.
// No `onBlur`: FieldError says why a submit-validated field leaves it out.
const TextField = ({ className, id, label, ...input }: TextFieldProps) => {
  const field = useFieldContext<string>();
  const generatedId = useId();
  const { errors } = field.state.meta;
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <Label htmlFor={id ?? generatedId}>{label}</Label>
      <Input
        {...input}
        aria-invalid={errors.length > 0 ? "true" : undefined}
        id={id ?? generatedId}
        name={field.name}
        onChange={(event) => field.handleChange(event.target.value)}
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
