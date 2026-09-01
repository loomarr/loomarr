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
  }) => void;
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
    onSave({ label: label.trim(), events, enabled, settings });
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
      onSuccess: () => void refresh(),
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
        <ProviderForm
          definition={definition}
          provider={provider}
          pending={update.isPending}
          onCancel={() => setEditing(false)}
          onSave={(value) => update.mutate({ id: provider.id, data: value })}
        />
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
    () =>
      typesQuery.data?.status === 200
        ? typesQuery.data.data.providers.filter((provider) => !provider.memberOwned)
        : [],
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
            <ProviderForm
              key={definition.type}
              definition={definition}
              pending={create.isPending}
              onCancel={() => setAdding(false)}
              onSave={(value) => create.mutate({ data: { type: definition.type as never, ...value } })}
            />
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
