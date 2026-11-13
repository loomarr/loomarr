import { NotificationProviderFieldDTOKind } from "@loomarr/api/models/notificationProviderFieldDTOKind";
import { Action, CompactSelect, Field, Stack, Text, Toggle } from "@loomarr/design-system";
import type { ComponentProps } from "react";
import type { ProviderFieldInputProps } from "./provider-field-input.type";

// The keyboard a destination's text field asks for, by the kind the server declares for it.
const KEYBOARD: Partial<
  Record<NotificationProviderFieldDTOKind, ComponentProps<typeof Field>["keyboardType"]>
> = {
  [NotificationProviderFieldDTOKind.number]: "numeric",
  [NotificationProviderFieldDTOKind.url]: "url",
};

/** One setting of a provider type, drawn for the kind the server declares: a toggle, a select, a text area or a text field. */
const ProviderFieldInput = ({
  field,
  inputId,
  value,
  configured,
  cleared,
  error,
  onChange,
  onClear,
}: ProviderFieldInputProps) => {
  const errorId = `${inputId}-error`;
  if (field.kind === "toggle") {
    return (
      <Toggle
        id={inputId}
        label={field.label}
        checked={value === "true"}
        onCheckedChange={(checked) => onChange(String(checked))}
      />
    );
  }
  return (
    <div className="grid gap-1.5">
      <Text as="label" htmlFor={inputId} textRole="label">
        {field.label}
        {field.required ? " *" : ""}
      </Text>
      {field.kind === "select" ? (
        <CompactSelect
          aria-describedby={error ? errorId : undefined}
          id={inputId}
          invalid={Boolean(error)}
          label={field.label}
          onValueChange={onChange}
          options={field.options ?? []}
          value={value}
        />
      ) : field.kind === "textarea" ? (
        <Field
          aria-describedby={error ? errorId : undefined}
          autoCapitalize="none"
          autoComplete={field.sensitive ? "new-password" : undefined}
          autoCorrect={false}
          disabled={cleared}
          id={inputId}
          invalid={Boolean(error)}
          multiline
          numberOfLines={4}
          onChangeText={onChange}
          spellCheck={false}
          value={value}
        />
      ) : (
        <Field
          aria-describedby={error ? errorId : undefined}
          autoCapitalize="none"
          autoComplete={field.sensitive ? "new-password" : undefined}
          autoCorrect={false}
          disabled={cleared}
          id={inputId}
          invalid={Boolean(error)}
          keyboardType={KEYBOARD[field.kind] ?? "default"}
          onChangeText={onChange}
          secureTextEntry={field.sensitive || field.kind === "password"}
          value={value}
        />
      )}
      {error ? (
        <div role="alert">
          <Text as="p" id={errorId} textRole="compact" tone="danger">
            {error}
          </Text>
        </div>
      ) : null}
      {field.sensitive && configured ? (
        <Stack align="center" direction="row" gap="inline" wrap>
          <Text as="span" textRole="hint" tone="secondary">
            {cleared ? "Will be cleared when saved." : "Configured — leave blank to keep it."}
          </Text>
          <Action onPress={onClear} variant="ghost">
            {cleared ? "Keep saved value" : `Clear ${field.label}`}
          </Action>
        </Stack>
      ) : null}
      {field.description ? (
        <Text as="p" textRole="hint" tone="secondary">
          {field.description}
        </Text>
      ) : null}
    </div>
  );
};

export { ProviderFieldInput };
