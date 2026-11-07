interface FieldErrorProps {
  // A TanStack Form field's `state.meta.errors`: Standard Schema issues, or undefined for a
  // validator that passed. Only the first is shown, so a field never stacks messages.
  errors: readonly ({ message: string } | undefined)[];
  // Layout only (a grid span). Colour and type are FieldHint's own.
  className?: string;
}

export type { FieldErrorProps };
