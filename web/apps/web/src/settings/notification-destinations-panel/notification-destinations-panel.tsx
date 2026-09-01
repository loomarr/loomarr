import * as notificationsApi from "@loomarr/api/endpoints/notifications";
import type { NotificationProviderDTO } from "@loomarr/api/models/notificationProviderDTO";
import type { NotificationProviderFieldDTO } from "@loomarr/api/models/notificationProviderFieldDTO";
import type { NotificationProviderTypeDTO } from "@loomarr/api/models/notificationProviderTypeDTO";
import { useQueryClient } from "@tanstack/react-query";
import { BellRing, Plus, X } from "lucide-react";
import { useMemo, useState } from "react";
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
  value,
  configured,
  cleared,
  onChange,
  onClear,
}: {
  field: NotificationProviderFieldDTO;
  value: string;
  configured: boolean;
  cleared: boolean;
  onChange: (value: string) => void;
  onClear: () => void;
}) => {
  if (field.kind === "toggle") {
    return (
      <label className="flex items-center gap-2 text-sm" htmlFor={`provider-field-${field.key}`}>
        <Checkbox
          id={`provider-field-${field.key}`}
          checked={value === "true"}
          onChange={(event) => onChange(event.target.checked ? "true" : "false")}
        />
        {field.label}
      </label>
    );
  }
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={`provider-field-${field.key}`}>
        {field.label}
        {field.required ? " *" : ""}
      </Label>
      {field.kind === "select" ? (
        <select
          id={`provider-field-${field.key}`}
          className="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
          value={value}
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
          id={`provider-field-${field.key}`}
          type={field.sensitive || field.kind === "password" ? "password" : field.kind}
          value={value}
          disabled={cleared}
          autoComplete={field.sensitive ? "new-password" : undefined}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
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
  onCancel,
  onSave,
}: {
  definition: NotificationProviderTypeDTO;
  provider?: NotificationProviderDTO;
  pending: boolean;
  onCancel: () => void;
  onSave: (value: {
    label: string;
    events: string[];
    enabled: boolean;
    settings: Record<string, string>;
  }) => void | Promise<void>;
}) => {
  const stored = useMemo(
    () => new Map(provider?.settings.map((setting) => [setting.key, setting])),
    [provider],
  );
  const [label, setLabel] = useState(provider?.label ?? definition.name);
  const [events, setEvents] = useState<string[]>(provider?.events ?? []);
  const [enabled, setEnabled] = useState(provider?.enabled ?? true);
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      definition.fields.map((field) => [
        field.key,
        field.sensitive ? "" : (stored.get(field.key)?.value ?? field.default ?? ""),
      ]),
    ),
  );
  const [clearedSecrets, setClearedSecrets] = useState<Set<string>>(new Set());

  const complete =
    label.trim() !== "" &&
    events.length > 0 &&
    definition.fields.every((field) => {
      if (!field.required) return true;
      if (!field.sensitive) return (values[field.key] ?? "").trim() !== "";
      return (
        (values[field.key] ?? "").trim() !== "" ||
        (stored.get(field.key)?.secretConfigured === true && !clearedSecrets.has(field.key))
      );
    });

  const submit = () => {
    const settings: Record<string, string> = {};
    for (const field of definition.fields) {
      const value = values[field.key] ?? "";
      if (field.sensitive) {
        if (clearedSecrets.has(field.key)) settings[field.key] = "";
        else if (value !== "") settings[field.key] = value;
      } else {
        settings[field.key] = value;
      }
    }
    void onSave({ label: label.trim(), events, enabled, settings });
  };

  return (
    <Card className="grid gap-5 p-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h3 className="font-semibold">{provider ? `Edit ${provider.label}` : `Add ${definition.name}`}</h3>
          <p className="mt-1 text-muted-foreground text-sm">
            Enter the provider settings, choose events, and save.
          </p>
        </div>
        <Button type="button" variant="ghost" size="sm" onClick={onCancel} aria-label="Close provider form">
          <X className="size-4" aria-hidden />
        </Button>
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="provider-label">Label *</Label>
        <Input id="provider-label" value={label} onChange={(event) => setLabel(event.target.value)} />
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        {definition.fields.map((field) => (
          <FieldInput
            key={field.key}
            field={field}
            value={values[field.key] ?? ""}
            configured={stored.get(field.key)?.secretConfigured === true}
            cleared={clearedSecrets.has(field.key)}
            onChange={(value) => {
              setValues((current) => ({ ...current, [field.key]: value }));
              setClearedSecrets((current) => {
                const next = new Set(current);
                next.delete(field.key);
                return next;
              });
            }}
            onClear={() =>
              setClearedSecrets((current) => {
                const next = new Set(current);
                if (next.has(field.key)) next.delete(field.key);
                else next.add(field.key);
                return next;
              })
            }
          />
        ))}
      </div>

      <fieldset className="grid gap-2 sm:grid-cols-2">
        <legend className="mb-2 font-medium text-sm">Events *</legend>
        {definition.events.map((event) => (
          <label className="flex items-center gap-2 text-sm" key={event} htmlFor={`provider-event-${event}`}>
            <Checkbox
              id={`provider-event-${event}`}
              checked={events.includes(event)}
              onChange={(input) =>
                setEvents((current) =>
                  input.target.checked ? [...current, event] : current.filter((item) => item !== event),
                )
              }
            />
            {words(event)}
          </label>
        ))}
      </fieldset>

      <label className="flex items-center gap-2 text-sm" htmlFor="provider-enabled">
        <Checkbox
          id="provider-enabled"
          checked={enabled}
          onChange={(event) => setEnabled(event.target.checked)}
        />
        Enable provider
      </label>

      <div className="flex flex-wrap gap-2">
        <Button disabled={!complete || pending} onClick={submit}>
          Save provider
        </Button>
        <Button type="button" variant="outline" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </Card>
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
  onCancel,
  onSave,
}: {
  definition: NotificationProviderTypeDTO;
  provider?: NotificationProviderDTO;
  publicKey?: string;
  pending: boolean;
  onCancel: () => void;
  onSave: (value: {
    label: string;
    events: string[];
    enabled: boolean;
    settings: Record<string, string>;
  }) => void | Promise<void>;
}) => {
  const [label, setLabel] = useState(provider?.label ?? "This browser");
  const [events, setEvents] = useState<string[]>(provider?.events ?? []);
  const [message, setMessage] = useState("");
  const [enabling, setEnabling] = useState(false);
  const supported =
    typeof window !== "undefined" &&
    "Notification" in window &&
    "serviceWorker" in navigator &&
    "PushManager" in window;

  const saveExisting = () =>
    onSave({ label: label.trim(), events, enabled: provider?.enabled ?? true, settings: {} });

  const enableBrowser = async () => {
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
        label: label.trim(),
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

  return (
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
      <div className="grid gap-1.5">
        <Label htmlFor="provider-label">Label *</Label>
        <Input id="provider-label" value={label} onChange={(event) => setLabel(event.target.value)} />
      </div>
      <fieldset className="grid gap-2 sm:grid-cols-2">
        <legend className="mb-2 font-medium text-sm">Events *</legend>
        {definition.events.map((event) => (
          <label className="flex items-center gap-2 text-sm" key={event} htmlFor={`provider-event-${event}`}>
            <Checkbox
              id={`provider-event-${event}`}
              checked={events.includes(event)}
              onChange={(input) =>
                setEvents((current) =>
                  input.target.checked ? [...current, event] : current.filter((item) => item !== event),
                )
              }
            />
            {words(event)}
          </label>
        ))}
      </fieldset>
      <p className="text-muted-foreground text-sm">
        Locked-screen previews say only that Loomarr has a new notification. Open Loomarr to see details.
      </p>
      <div className="flex flex-wrap gap-2">
        <Button
          disabled={!label.trim() || events.length === 0 || pending || enabling}
          onClick={() => void (provider ? saveExisting() : enableBrowser())}
        >
          {provider ? "Save provider" : "Enable this browser"}
        </Button>
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
  );
};

const ProviderRow = ({
  provider,
  definition,
}: {
  provider: NotificationProviderDTO;
  definition?: NotificationProviderTypeDTO;
}) => {
  const refresh = useRefresh();
  const [editing, setEditing] = useState(false);
  const [message, setMessage] = useState("");
  const update = notificationsApi.useNotificationProvidersUpdate({
    mutation: {
      onSuccess: () => {
        setEditing(false);
        setMessage("Provider saved.");
        void refresh();
      },
      onError: () => setMessage("Loomarr could not save this provider."),
    },
  });
  const remove = notificationsApi.useNotificationProvidersDelete({
    mutation: {
      onSuccess: () => {
        if (provider.type === "web_push" && "serviceWorker" in navigator) {
          void navigator.serviceWorker
            .getRegistration()
            .then((registration) => registration?.pushManager.getSubscription())
            .then((subscription) => subscription?.unsubscribe());
        }
        void refresh();
      },
      onError: () => setMessage("Loomarr could not delete this provider."),
    },
  });
  const test = notificationsApi.useNotificationProvidersTest({
    mutation: {
      onSuccess: (response) => {
        if (response.status === 202) setMessage(response.data.hint);
        void refresh();
      },
      onError: () => setMessage("Loomarr could not queue the provider test."),
    },
  });

  if (editing && definition) {
    return (
      <li>
        {provider.type === "web_push" ? (
          <WebPushForm
            definition={definition}
            provider={provider}
            pending={update.isPending}
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
            onCancel={() => setEditing(false)}
            onSave={(value) => update.mutate({ id: provider.id, data: value })}
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
            <Button
              variant="ghost"
              size="sm"
              disabled={remove.isPending}
              onClick={() => remove.mutate({ id: provider.id })}
            >
              Delete
            </Button>
          </div>
        </div>
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
      onError: () => setMessage("Loomarr could not save this provider."),
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
                onCancel={() => setAdding(false)}
                onSave={(value) => create.mutate({ data: { type: definition.type as never, ...value } })}
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
