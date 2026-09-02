import * as notificationsApi from "@loomarr/api/endpoints/notifications";
import type { NotificationProviderDTO } from "@loomarr/api/models/notificationProviderDTO";
import type { NotificationProviderFieldDTO } from "@loomarr/api/models/notificationProviderFieldDTO";
import type { NotificationProviderTypeDTO } from "@loomarr/api/models/notificationProviderTypeDTO";
import { notificationProviderFormSchema } from "@loomarr/core/schemas";
import { useForm } from "@tanstack/react-form";
import { useQueryClient } from "@tanstack/react-query";
import { BellRing, Plus, X } from "lucide-react";
import { useId, useMemo, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const words = (value: string) =>
  value
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");

const useRefresh = () => {
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: notificationsApi.getNotificationProvidersListQueryKey() });
};

const FieldInput = ({
  field,
  inputId,
  value,
  configured,
  cleared,
  error,
  onChange,
  onClear,
}: {
  field: NotificationProviderFieldDTO;
  inputId: string;
  value: string;
  configured: boolean;
  cleared: boolean;
  error?: string;
  onChange: (value: string) => void;
  onClear: () => void;
}) => {
  const errorId = `${inputId}-error`;
  if (field.kind === "toggle") {
    return (
      <label className="flex items-center gap-2 text-sm" htmlFor={inputId}>
        <Checkbox
          id={inputId}
          checked={value === "true"}
          onChange={(event) => onChange(event.target.checked ? "true" : "false")}
        />
        {field.label}
      </label>
    );
  }
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={inputId}>
        {field.label}
        {field.required ? " *" : ""}
      </Label>
      {field.kind === "select" ? (
        <select
          id={inputId}
          className="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
          value={value}
          aria-invalid={error ? "true" : undefined}
          aria-describedby={error ? errorId : undefined}
          onChange={(event) => onChange(event.target.value)}
        >
          {(field.options ?? []).map((option) => (
            <option value={option.value} key={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      ) : (
        <Input
          id={inputId}
          type={field.sensitive || field.kind === "password" ? "password" : field.kind}
          value={value}
          disabled={cleared}
          autoComplete={field.sensitive ? "new-password" : undefined}
          aria-invalid={error ? "true" : undefined}
          aria-describedby={error ? errorId : undefined}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
      {error ? (
        <p id={errorId} className="text-onair-300 text-sm" role="alert">
          {error}
        </p>
      ) : null}
      {field.sensitive && configured ? (
        <div className="flex flex-wrap items-center gap-2 text-muted-foreground text-xs">
          <span>{cleared ? "Will be cleared when saved." : "Configured — leave blank to keep it."}</span>
          <Button type="button" variant="ghost" size="sm" onClick={onClear}>
            {cleared ? "Keep saved value" : `Clear ${field.label}`}
          </Button>
        </div>
      ) : null}
      {field.description ? <p className="text-muted-foreground text-xs">{field.description}</p> : null}
    </div>
  );
};

const ProviderForm = ({
  definition,
  provider,
  pending,
  error,
  onCancel,
  onSave,
}: {
  definition: NotificationProviderTypeDTO;
  provider?: NotificationProviderDTO;
  pending: boolean;
  error?: unknown;
  onCancel: () => void;
  onSave: (value: {
    label: string;
    events: string[];
    enabled: boolean;
    settings: Record<string, string>;
  }) => Promise<void>;
}) => {
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
  const form = useForm({
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
      <Card className="grid gap-5 p-5">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h3 className="font-semibold">
              {provider ? `Edit ${provider.label}` : `Add ${definition.name}`}
            </h3>
            <p className="mt-1 text-muted-foreground text-sm">
              Enter the provider settings, choose events, and save.
            </p>
          </div>
          <Button type="button" variant="ghost" size="sm" onClick={onCancel} aria-label="Close provider form">
            <X className="size-4" aria-hidden />
          </Button>
        </div>

        {error != null ? <ErrorState error={error} /> : null}

        <form.Field name="label">
          {(field) => {
            const fieldError = field.state.meta.errors[0]?.message;
            const inputId = `${idPrefix}-provider-label`;
            return (
              <div className="grid gap-1.5">
                <Label htmlFor={inputId}>Label *</Label>
                <Input
                  id={inputId}
                  name={field.name}
                  value={field.state.value}
                  aria-invalid={fieldError ? "true" : undefined}
                  aria-describedby={fieldError ? `${inputId}-error` : undefined}
                  onBlur={field.handleBlur}
                  onChange={(event) => field.handleChange(event.target.value)}
                />
                {fieldError ? (
                  <p id={`${inputId}-error`} className="text-onair-300 text-sm" role="alert">
                    {fieldError}
                  </p>
                ) : null}
              </div>
            );
          }}
        </form.Field>

        <div className="grid gap-4 sm:grid-cols-2">
          {definition.fields.map((field) => (
            <form.Field name={`settings.${field.key}`} key={field.key}>
              {(settingField) => (
                <form.Subscribe selector={(state) => state.values.clearedSecrets}>
                  {(clearedSecrets) => (
                    <FieldInput
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
              <legend className="mb-2 font-medium text-sm">Events *</legend>
              {definition.events.map((event) => {
                const inputId = `${idPrefix}-provider-event-${event}`;
                return (
                  <label className="flex items-center gap-2 text-sm" key={event} htmlFor={inputId}>
                    <Checkbox
                      id={inputId}
                      checked={field.state.value.includes(event)}
                      onChange={(input) =>
                        field.handleChange(
                          input.target.checked
                            ? [...field.state.value, event]
                            : field.state.value.filter((item) => item !== event),
                        )
                      }
                    />
                    {words(event)}
                  </label>
                );
              })}
              {field.state.meta.errors[0] ? (
                <p className="text-onair-300 text-sm sm:col-span-2" role="alert">
                  {field.state.meta.errors[0].message}
                </p>
              ) : null}
            </fieldset>
          )}
        </form.Field>

        <form.Field name="enabled">
          {(field) => {
            const inputId = `${idPrefix}-provider-enabled`;
            return (
              <label className="flex items-center gap-2 text-sm" htmlFor={inputId}>
                <Checkbox
                  id={inputId}
                  checked={field.state.value}
                  onChange={(event) => field.handleChange(event.target.checked)}
                />
                Enable provider
              </label>
            );
          }}
        </form.Field>

        <div className="flex flex-wrap gap-2">
          <form.Subscribe selector={(state) => state.isSubmitting}>
            {(submitting) => (
              <Button type="submit" disabled={pending || submitting}>
                Save provider
              </Button>
            )}
          </form.Subscribe>
          <Button type="button" variant="outline" onClick={onCancel}>
            Cancel
          </Button>
        </div>
      </Card>
    </form>
  );
};

const decodeApplicationServerKey = (value: string) => {
  const padded = `${value.replaceAll("-", "+").replaceAll("_", "/")}${"=".repeat((4 - (value.length % 4)) % 4)}`;
  return Uint8Array.from(atob(padded), (character) => character.charCodeAt(0));
};

const WebPushForm = ({
  definition,
  provider,
  publicKey,
  pending,
  error,
  onCancel,
  onSave,
}: {
  definition: NotificationProviderTypeDTO;
  provider?: NotificationProviderDTO;
  publicKey?: string;
  pending: boolean;
  error?: unknown;
  onCancel: () => void;
  onSave: (value: {
    label: string;
    events: string[];
    enabled: boolean;
    settings: Record<string, string>;
  }) => Promise<void>;
}) => {
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

  const form = useForm({
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
      <Card className="grid gap-5 p-5">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h3 className="font-semibold">{provider ? `Edit ${provider.label}` : "Add Browser Push"}</h3>
            <p className="mt-1 text-muted-foreground text-sm">
              Loomarr asks this browser for permission only when you enable it below.
            </p>
          </div>
          <Button type="button" variant="ghost" size="sm" onClick={onCancel} aria-label="Close provider form">
            <X className="size-4" aria-hidden />
          </Button>
        </div>
        {error != null ? <ErrorState error={error} /> : null}
        <form.Field name="label">
          {(field) => {
            const inputId = `${idPrefix}-provider-label`;
            const fieldError = field.state.meta.errors[0]?.message;
            return (
              <div className="grid gap-1.5">
                <Label htmlFor={inputId}>Label *</Label>
                <Input
                  id={inputId}
                  name={field.name}
                  value={field.state.value}
                  aria-invalid={fieldError ? "true" : undefined}
                  aria-describedby={fieldError ? `${inputId}-error` : undefined}
                  onBlur={field.handleBlur}
                  onChange={(event) => field.handleChange(event.target.value)}
                />
                {fieldError ? (
                  <p id={`${inputId}-error`} className="text-onair-300 text-sm" role="alert">
                    {fieldError}
                  </p>
                ) : null}
              </div>
            );
          }}
        </form.Field>
        <form.Field name="events">
          {(field) => (
            <fieldset className="grid gap-2 sm:grid-cols-2">
              <legend className="mb-2 font-medium text-sm">Events *</legend>
              {definition.events.map((event) => {
                const inputId = `${idPrefix}-provider-event-${event}`;
                return (
                  <label className="flex items-center gap-2 text-sm" key={event} htmlFor={inputId}>
                    <Checkbox
                      id={inputId}
                      checked={field.state.value.includes(event)}
                      onChange={(input) =>
                        field.handleChange(
                          input.target.checked
                            ? [...field.state.value, event]
                            : field.state.value.filter((item) => item !== event),
                        )
                      }
                    />
                    {words(event)}
                  </label>
                );
              })}
              {field.state.meta.errors[0] ? (
                <p className="text-onair-300 text-sm sm:col-span-2" role="alert">
                  {field.state.meta.errors[0].message}
                </p>
              ) : null}
            </fieldset>
          )}
        </form.Field>
        <p className="text-muted-foreground text-sm">
          Locked-screen previews say only that Loomarr has a new notification. Open Loomarr to see details.
        </p>
        <div className="flex flex-wrap gap-2">
          <form.Subscribe selector={(state) => state.isSubmitting}>
            {(submitting) => (
              <Button type="submit" disabled={pending || enabling || submitting}>
                {provider ? "Save provider" : "Enable this browser"}
              </Button>
            )}
          </form.Subscribe>
          <Button type="button" variant="outline" onClick={onCancel}>
            Cancel
          </Button>
        </div>
        {message ? (
          <p className="text-sm" role={message.includes("could not") ? "alert" : "status"} aria-live="polite">
            {message}
          </p>
        ) : null}
      </Card>
    </form>
  );
};

const ProviderRow = ({
  provider,
  definition,
}: {
  provider: NotificationProviderDTO;
  definition?: NotificationProviderTypeDTO;
}) => {
  const idPrefix = useId();
  const refresh = useRefresh();
  const [editing, setEditing] = useState(false);
  const [message, setMessage] = useState("");
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
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
    setDeleteConfirmation("");
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
      <Card className="p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="font-medium">{provider.label}</h3>
              <Badge variant={provider.enabled ? "signal" : "neutral"}>
                {provider.enabled ? "Enabled" : "Disabled"}
              </Badge>
              <Badge variant="neutral">{definition?.name ?? words(provider.type)}</Badge>
            </div>
            <p className="mt-1 text-muted-foreground text-sm">{provider.events.map(words).join(", ")}</p>
            {provider.health ? (
              <p className="mt-2 text-muted-foreground text-xs">
                {provider.health.queuedCount} queued · {provider.health.terminalFailureCount} failed
                {provider.health.lastFailureOutcome
                  ? ` · Last failure: ${words(provider.health.lastFailureOutcome)}`
                  : ""}
              </p>
            ) : null}
          </div>
          {!confirmingDelete ? (
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
                Edit
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={!provider.enabled || test.isPending}
                onClick={() => test.mutate({ id: provider.id, data: { requestId: crypto.randomUUID() } })}
              >
                Send test
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={update.isPending}
                onClick={() =>
                  update.mutate({
                    id: provider.id,
                    data: { label: provider.label, events: provider.events, enabled: !provider.enabled },
                  })
                }
              >
                {provider.enabled ? "Disable" : "Enable"}
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setConfirmingDelete(true)}>
                Delete
              </Button>
            </div>
          ) : null}
        </div>
        {confirmingDelete ? (
          <div className="mt-4 grid gap-3 rounded-md border border-onair-tint-15 bg-onair-tint-15 p-3">
            <div>
              <p className="text-sm">Delete {provider.label} and stop sending its notifications?</p>
              <p className="mt-1 text-onair-300 text-xs">This can't be undone.</p>
            </div>
            <div className="grid max-w-sm gap-1.5">
              <Label htmlFor={`${idPrefix}-delete-confirmation`}>Type {provider.label} to confirm</Label>
              <Input
                id={`${idPrefix}-delete-confirmation`}
                value={deleteConfirmation}
                autoComplete="off"
                onChange={(event) => setDeleteConfirmation(event.target.value)}
              />
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="destructive"
                size="sm"
                disabled={deleteConfirmation !== provider.label || remove.isPending}
                onClick={() => void deleteProvider().catch(() => undefined)}
              >
                Delete provider
              </Button>
              <Button
                variant="ghost"
                size="sm"
                disabled={remove.isPending}
                onClick={() => {
                  setConfirmingDelete(false);
                  setDeleteConfirmation("");
                }}
              >
                Cancel
              </Button>
            </div>
          </div>
        ) : null}
        {update.error != null || remove.error != null || test.error != null ? (
          <ErrorState className="mt-3" error={update.error ?? remove.error ?? test.error} />
        ) : null}
        {message ? (
          <p className="mt-3 text-sm" role="status" aria-live="polite">
            {message}
          </p>
        ) : null}
      </Card>
    </li>
  );
};

const NotificationDestinationsPanel = () => {
  const typesQuery = notificationsApi.useNotificationProviderTypesList();
  const providersQuery = notificationsApi.useNotificationProvidersList();
  const refresh = useRefresh();
  const [adding, setAdding] = useState(false);
  const [selectedType, setSelectedType] = useState("email");
  const [message, setMessage] = useState("");
  const definitions = useMemo(
    () => (typesQuery.data?.status === 200 ? typesQuery.data.data.providers : []),
    [typesQuery.data],
  );
  const providers = providersQuery.data?.status === 200 ? providersQuery.data.data.providers : [];
  const definition = definitions.find((item) => item.type === selectedType) ?? definitions[0];
  const create = notificationsApi.useNotificationProvidersCreate({
    mutation: {
      onSuccess: () => {
        setAdding(false);
        setMessage("Provider saved. You can send a test now.");
        void refresh();
      },
    },
  });

  return (
    <section className="grid gap-4" aria-labelledby="notification-providers-title">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-signal/10 text-signal">
            <BellRing className="size-4" aria-hidden />
          </span>
          <div>
            <h2 id="notification-providers-title" className="font-semibold text-lg">
              Notification providers
            </h2>
            <p className="mt-1 text-muted-foreground text-sm">
              Add SMTP, Slack, Discord, or another provider, then choose the events it receives.
            </p>
          </div>
        </div>
        {!adding ? (
          <Button onClick={() => setAdding(true)}>
            <Plus className="size-4" aria-hidden /> Add provider
          </Button>
        ) : null}
      </div>

      {providersQuery.isLoading || typesQuery.isLoading ? (
        <p className="text-muted-foreground text-sm">Loading providers…</p>
      ) : null}
      {providersQuery.isError || typesQuery.isError ? (
        <p className="text-destructive text-sm" role="alert">
          Providers could not be loaded.
        </p>
      ) : null}
      {!providersQuery.isLoading && providers.length === 0 ? (
        <p className="rounded-lg border border-border border-dashed p-4 text-muted-foreground text-sm">
          No notification providers yet.
        </p>
      ) : null}

      <ul className="grid gap-3">
        {providers.map((provider) => (
          <ProviderRow
            provider={provider}
            definition={definitions.find((item) => item.type === provider.type)}
            key={provider.id}
          />
        ))}
      </ul>

      {adding ? (
        <div className="grid gap-4">
          <div className="grid max-w-sm gap-1.5">
            <Label htmlFor="provider-type">Provider</Label>
            <select
              id="provider-type"
              className="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
              value={definition?.type ?? ""}
              onChange={(event) => setSelectedType(event.target.value)}
            >
              {definitions.map((item) => (
                <option value={item.type} key={item.type}>
                  {item.name}
                </option>
              ))}
            </select>
          </div>
          {definition ? (
            definition.type === "web_push" ? (
              <WebPushForm
                key={definition.type}
                definition={definition}
                publicKey={
                  typesQuery.data?.status === 200 ? typesQuery.data.data.webPushPublicKey : undefined
                }
                pending={create.isPending}
                error={create.error}
                onCancel={() => setAdding(false)}
                onSave={async (value) => {
                  await create.mutateAsync({ data: { type: definition.type as never, ...value } });
                }}
              />
            ) : (
              <ProviderForm
                key={definition.type}
                definition={definition}
                pending={create.isPending}
                error={create.error}
                onCancel={() => setAdding(false)}
                onSave={async (value) => {
                  await create.mutateAsync({ data: { type: definition.type as never, ...value } });
                }}
              />
            )
          ) : null}
        </div>
      ) : null}

      {message ? (
        <p className="text-sm" role="status" aria-live="polite">
          {message}
        </p>
      ) : null}
    </section>
  );
};

export { NotificationDestinationsPanel };
