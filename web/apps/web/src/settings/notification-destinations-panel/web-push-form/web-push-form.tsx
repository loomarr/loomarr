import { notificationProviderFormSchema } from "@loomarr/core/schemas";
import { Action, Card, Stack, Text, Toggle } from "@loomarr/design-system";
import { useId, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useAppForm } from "@/lib/app-form";
import { decodeApplicationServerKey } from "../decode-application-server-key";
import { words } from "../provider-words";
import type { WebPushFormProps } from "./web-push-form.type";

/** Adds or edits this browser's Push destination; the browser's permission is asked for only when it is enabled here. */
const WebPushForm = ({
  definition,
  provider,
  publicKey,
  pending,
  error,
  onCancel,
  onSave,
}: WebPushFormProps) => {
  const idPrefix = useId();
  const [message, setMessage] = useState("");
  const [enabling, setEnabling] = useState(false);
  const supported =
    typeof window !== "undefined" &&
    "Notification" in window &&
    "serviceWorker" in navigator &&
    "PushManager" in window;

  const enableBrowser = async (label: string, events: string[]) => {
    if (!supported || !publicKey) {
      setMessage("Browser notifications are not available in this browser or connection.");
      return;
    }
    setEnabling(true);
    setMessage("");
    let subscription: PushSubscription | undefined;
    try {
      const registration = await navigator.serviceWorker.register("/push-worker.js");
      const permission =
        Notification.permission === "default"
          ? await Notification.requestPermission()
          : Notification.permission;
      if (permission !== "granted") {
        setMessage("Notification permission was not granted. Nothing else in Loomarr is affected.");
        return;
      }
      subscription =
        (await registration.pushManager.getSubscription()) ??
        (await registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: decodeApplicationServerKey(publicKey),
        }));
      const value = subscription.toJSON();
      if (!value.endpoint || !value.keys?.p256dh || !value.keys.auth) {
        throw new Error("The browser returned an incomplete Push subscription.");
      }
      await onSave({
        label,
        events,
        enabled: true,
        settings: { endpoint: value.endpoint, p256dh: value.keys.p256dh, auth: value.keys.auth },
      });
    } catch {
      if (!provider && subscription) await subscription.unsubscribe().catch(() => false);
      setMessage("Loomarr could not enable notifications for this browser.");
    } finally {
      setEnabling(false);
    }
  };

  const form = useAppForm({
    defaultValues: {
      label: provider?.label ?? "This browser",
      events: provider?.events ?? [],
      enabled: provider?.enabled ?? true,
      settings: {} as Record<string, string>,
      clearedSecrets: [] as string[],
    },
    validators: { onSubmit: notificationProviderFormSchema([]) },
    onSubmit: async ({ value }) => {
      const label = value.label.trim();
      if (provider) {
        await onSave({ label, events: value.events, enabled: value.enabled, settings: {} });
      } else {
        await enableBrowser(label, value.events);
      }
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
              {provider ? `Edit ${provider.label}` : "Add Browser Push"}
            </Text>
            <Text as="p" className="mt-1" textRole="compact" tone="secondary">
              Loomarr asks this browser for permission only when you enable it below.
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
        <Text as="p" textRole="compact" tone="secondary">
          Locked-screen previews say only that Loomarr has a new notification. Open Loomarr to see details.
        </Text>
        <Stack direction="row" gap="inline" wrap>
          <form.Subscribe selector={(state) => state.isSubmitting}>
            {(submitting) => (
              <Action disabled={pending || enabling || submitting} submit>
                {provider ? "Save provider" : "Enable this browser"}
              </Action>
            )}
          </form.Subscribe>
          <Action onPress={onCancel} variant="outline">
            Cancel
          </Action>
        </Stack>
        {message ? (
          <div role={message.includes("could not") ? "alert" : "status"} aria-live="polite">
            <Text as="p" textRole="compact">
              {message}
            </Text>
          </div>
        ) : null}
      </Card>
    </form>
  );
};

export { WebPushForm };
