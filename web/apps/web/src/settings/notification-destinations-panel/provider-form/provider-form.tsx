import { notificationProviderFormSchema } from "@loomarr/core/schemas";
import { Action, Card, Stack, Text, Toggle } from "@loomarr/design-system";
import { useId, useMemo } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useAppForm } from "@/lib/app-form";
import { ProviderFieldInput } from "../provider-field-input";
import { words } from "../provider-words";
import type { ProviderFormProps } from "./provider-form.type";

/** Adds or edits one provider: its label, the settings its type declares, the events it receives and whether it is on. */
const ProviderForm = ({ definition, provider, pending, error, onCancel, onSave }: ProviderFormProps) => {
  const idPrefix = useId();
  const stored = useMemo(
    () => new Map(provider?.settings.map((setting) => [setting.key, setting])),
    [provider],
  );
  const schema = useMemo(
    () =>
      notificationProviderFormSchema(
        definition.fields.filter((field) => field.required).map(({ key, label }) => ({ key, label })),
        definition.fields
          .filter((field) => field.sensitive && stored.get(field.key)?.secretConfigured === true)
          .map((field) => field.key),
      ),
    [definition.fields, stored],
  );
  const form = useAppForm({
    defaultValues: {
      label: provider?.label ?? definition.name,
      events: provider?.events ?? [],
      enabled: provider?.enabled ?? true,
      settings: Object.fromEntries(
        definition.fields.map((field) => [
          field.key,
          field.sensitive ? "" : (stored.get(field.key)?.value ?? field.default ?? ""),
        ]),
      ),
      clearedSecrets: [] as string[],
    },
    validators: { onSubmit: schema },
    onSubmit: async ({ value }) => {
      const settings: Record<string, string> = {};
      for (const field of definition.fields) {
        const setting = value.settings[field.key] ?? "";
        if (field.sensitive) {
          if (value.clearedSecrets.includes(field.key)) settings[field.key] = "";
          else if (setting !== "") settings[field.key] = setting;
        } else {
          settings[field.key] = setting;
        }
      }
      await onSave({
        label: value.label.trim(),
        events: value.events,
        enabled: value.enabled,
        settings,
      });
    },
  });

  return (
    <form
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        void form.handleSubmit().catch(() => undefined);
      }}
    >
      <Card gap={20} padding={20}>
        <Stack align="start" direction="row" gap="control" justify="between">
          <div>
            <Text as="h3" strong textRole="heading">
              {provider ? `Edit ${provider.label}` : `Add ${definition.name}`}
            </Text>
            <Text as="p" className="mt-1" textRole="compact" tone="secondary">
              Enter the provider settings, choose events, and save.
            </Text>
          </div>
          <Action icon="close" layout="icon" onPress={onCancel} variant="ghost">
            Close provider form
          </Action>
        </Stack>

        {error != null ? <ErrorState error={error} /> : null}

        <form.AppField name="label">
          {(field) => (
            <field.TextField autoComplete="off" id={`${idPrefix}-provider-label`} label="Label *" />
          )}
        </form.AppField>

        <div className="grid gap-4 sm:grid-cols-2">
          {definition.fields.map((field) => (
            <form.Field name={`settings.${field.key}`} key={field.key}>
              {(settingField) => (
                <form.Subscribe selector={(state) => state.values.clearedSecrets}>
                  {(clearedSecrets) => (
                    <ProviderFieldInput
                      field={field}
                      inputId={`${idPrefix}-provider-field-${field.key}`}
                      value={settingField.state.value}
                      configured={stored.get(field.key)?.secretConfigured === true}
                      cleared={clearedSecrets.includes(field.key)}
                      error={settingField.state.meta.errors[0]?.message}
                      onChange={(value) => {
                        settingField.handleChange(value);
                        form.setFieldValue("clearedSecrets", (current) =>
                          current.filter((key) => key !== field.key),
                        );
                      }}
                      onClear={() =>
                        form.setFieldValue("clearedSecrets", (current) =>
                          current.includes(field.key)
                            ? current.filter((key) => key !== field.key)
                            : [...current, field.key],
                        )
                      }
                    />
                  )}
                </form.Subscribe>
              )}
            </form.Field>
          ))}
        </div>

        <form.Field name="events">
          {(field) => (
            <fieldset className="grid gap-2 sm:grid-cols-2">
              <Text as="legend" className="mb-2" textRole="cardLabel">
                Events *
              </Text>
              {definition.events.map((event) => {
                const inputId = `${idPrefix}-provider-event-${event}`;
                return (
                  <Toggle
                    key={event}
                    id={inputId}
                    label={words(event)}
                    checked={field.state.value.includes(event)}
                    onCheckedChange={(checked) =>
                      field.handleChange(
                        checked
                          ? [...field.state.value, event]
                          : field.state.value.filter((item) => item !== event),
                      )
                    }
                  />
                );
              })}
              {field.state.meta.errors[0] ? (
                <div className="sm:col-span-2" role="alert">
                  <Text as="p" textRole="compact" tone="danger">
                    {field.state.meta.errors[0].message}
                  </Text>
                </div>
              ) : null}
            </fieldset>
          )}
        </form.Field>

        <form.Field name="enabled">
          {(field) => {
            const inputId = `${idPrefix}-provider-enabled`;
            return (
              <Toggle
                id={inputId}
                label="Enable provider"
                checked={field.state.value}
                onCheckedChange={field.handleChange}
              />
            );
          }}
        </form.Field>

        <Stack direction="row" gap="inline" wrap>
          <form.Subscribe selector={(state) => state.isSubmitting}>
            {(submitting) => (
              <Action disabled={pending || submitting} submit>
                Save provider
              </Action>
            )}
          </form.Subscribe>
          <Action onPress={onCancel} variant="outline">
            Cancel
          </Action>
        </Stack>
      </Card>
    </form>
  );
};

export { ProviderForm };
