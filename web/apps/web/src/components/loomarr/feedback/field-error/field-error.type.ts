interface FieldErrorProps {
  // A TanStack Form field's `state.meta.errors`: Standard Schema issues, or undefined for a
  // validator that passed. Only the first is shown, so a field never stacks messages.
  errors: readonly ({ message: string } | undefined)[];
}

export type { FieldErrorProps };
