import * as notificationsApi from "@loomarr/api/endpoints/notifications";
import { Action, Card, Field, Stack, StatusLabel, Text } from "@loomarr/design-system";
import { useId, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useAppForm } from "@/lib/app-form";
import { ProviderForm } from "../provider-form";
import { words } from "../provider-words";
import { useRefreshProviders } from "../use-refresh-providers";
import { WebPushForm } from "../web-push-form";
import type { ProviderRowProps } from "./provider-row.type";

/** One configured provider: its state and health, and what can be done to it (edit, test, switch, delete). */
const ProviderRow = ({ provider, definition }: ProviderRowProps) => {
  const idPrefix = useId();
  const refresh = useRefreshProviders();
  const [editing, setEditing] = useState(false);
  const [message, setMessage] = useState("");
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  // Deleting asks for the provider's name, typed back, before the button arms.
  const deleteForm = useAppForm({ defaultValues: { confirmation: "" } });
  const update = notificationsApi.useNotificationProvidersUpdate({
    mutation: {
      onSuccess: () => {
        setEditing(false);
        setMessage("Provider saved.");
        void refresh();
      },
    },
  });
  const remove = notificationsApi.useNotificationProvidersDelete();
  const test = notificationsApi.useNotificationProvidersTest({
    mutation: {
      onSuccess: (response) => {
        if (response.status === 202) setMessage(response.data.hint);
        void refresh();
      },
    },
  });

  const deleteProvider = async () => {
    let currentSubscription: PushSubscription | null | undefined;
    if (provider.type === "web_push" && "serviceWorker" in navigator) {
      currentSubscription = await navigator.serviceWorker
        .getRegistration()
        .then((registration) => registration?.pushManager.getSubscription())
        .catch(() => undefined);
    }
    const response = await remove.mutateAsync({
      id: provider.id,
      data: currentSubscription?.endpoint
        ? { currentBrowserEndpoint: currentSubscription.endpoint }
        : undefined,
    });
    if (response.status === 200 && response.data.unsubscribeCurrentBrowser) {
      await currentSubscription?.unsubscribe().catch(() => false);
    }
    setConfirmingDelete(false);
    deleteForm.reset();
    await refresh();
  };

  if (editing && definition) {
    return (
      <li>
        {provider.type === "web_push" ? (
          <WebPushForm
            definition={definition}
            provider={provider}
            pending={update.isPending}
            error={update.error}
            onCancel={() => setEditing(false)}
            onSave={async (value) => {
              await update.mutateAsync({ id: provider.id, data: value });
            }}
          />
        ) : (
          <ProviderForm
            definition={definition}
            provider={provider}
            pending={update.isPending}
            error={update.error}
            onCancel={() => setEditing(false)}
            onSave={async (value) => {
              await update.mutateAsync({ id: provider.id, data: value });
            }}
          />
        )}
      </li>
    );
  }

  return (
    <li>
      <Card padding={16}>
        <Stack align="start" direction="row" gap="control" justify="between" wrap>
          <div>
            <Stack align="center" direction="row" gap="inline" wrap>
              <Text as="h3" textRole="heading">
                {provider.label}
              </Text>
              <StatusLabel tone={provider.enabled ? "ok" : "off"}>
                {provider.enabled ? "Enabled" : "Disabled"}
              </StatusLabel>
              <Text textRole="compact" tone="secondary">
                {definition?.name ?? words(provider.type)}
              </Text>
            </Stack>
            <Text as="p" className="mt-1" textRole="compact" tone="secondary">
              {provider.events.map(words).join(", ")}
            </Text>
            {provider.health ? (
              <Text as="p" className="mt-2" textRole="hint" tone="secondary">
                {provider.health.queuedCount} queued · {provider.health.terminalFailureCount} failed
                {provider.health.lastFailureOutcome
                  ? ` · Last failure: ${words(provider.health.lastFailureOutcome)}`
                  : ""}
              </Text>
            ) : null}
          </div>
          {!confirmingDelete ? (
            <Stack direction="row" gap="inline" wrap>
              <Action onPress={() => setEditing(true)} variant="outline">
                Edit
              </Action>
              <Action
                disabled={!provider.enabled || test.isPending}
                onPress={() => test.mutate({ id: provider.id, data: { requestId: crypto.randomUUID() } })}
                variant="outline"
              >
                Send test
              </Action>
              <Action
                disabled={update.isPending}
                onPress={() =>
                  update.mutate({
                    id: provider.id,
                    data: { label: provider.label, events: provider.events, enabled: !provider.enabled },
                  })
                }
                variant="outline"
              >
                {provider.enabled ? "Disable" : "Enable"}
              </Action>
              <Action onPress={() => setConfirmingDelete(true)} variant="ghost">
                Delete
              </Action>
            </Stack>
          ) : null}
        </Stack>
        {confirmingDelete ? (
          <div className="border-onair-tint-15 bg-onair-tint-15 mt-4 grid gap-3 rounded-md border p-3">
            <div>
              <Text as="p" textRole="compact">
                Delete {provider.label} and stop sending its notifications?
              </Text>
              <Text as="p" className="mt-1" textRole="hint" tone="danger">
                This can't be undone.
              </Text>
            </div>
            <div className="grid max-w-sm gap-1.5">
              <deleteForm.Field name="confirmation">
                {(field) => (
                  <Field
                    autoCapitalize="none"
                    autoComplete="off"
                    autoCorrect={false}
                    id={`${idPrefix}-delete-confirmation`}
                    label={`Type ${provider.label} to confirm`}
                    onChangeText={field.handleChange}
                    value={field.state.value}
                  />
                )}
              </deleteForm.Field>
            </div>
            <Stack direction="row" flexShrink={1} gap="inline" wrap>
              <deleteForm.Subscribe selector={(state) => state.values.confirmation}>
                {(confirmation) => (
                  <Action
                    disabled={confirmation !== provider.label || remove.isPending}
                    onPress={() => void deleteProvider().catch(() => undefined)}
                    tone="danger"
                    variant="outline"
                  >
                    Delete provider
                  </Action>
                )}
              </deleteForm.Subscribe>
              <Action
                disabled={remove.isPending}
                onPress={() => {
                  setConfirmingDelete(false);
                  deleteForm.reset();
                }}
                variant="ghost"
              >
                Cancel
              </Action>
            </Stack>
          </div>
        ) : null}
        {update.error != null || remove.error != null || test.error != null ? (
          <ErrorState className="mt-3" error={update.error ?? remove.error ?? test.error} />
        ) : null}
        {message ? (
          <div className="mt-3" role="status" aria-live="polite">
            <Text as="p" textRole="compact">
              {message}
            </Text>
          </div>
        ) : null}
      </Card>
    </li>
  );
};

export { ProviderRow };
