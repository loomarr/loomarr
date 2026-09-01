import * as notificationsApi from "@loomarr/api/endpoints/notifications";
import type { NotificationDestinationDTO } from "@loomarr/api/models/notificationDestinationDTO";
import { useQueryClient } from "@tanstack/react-query";
import { BellRing, Plus } from "lucide-react";
import { useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type DestinationScope = "installation" | "person";
type Audience = "person" | "approvers" | "operators";

const installationProviders = [
  "webhook",
  "discord",
  "ntfy",
  "gotify",
  "apprise",
  "pushover",
  "telegram",
  "mattermost",
  "matrix",
  "mqtt",
  "slack",
] as const;
const personalProviders = ["email", "web_push"] as const;

const topics = [
  ["proposal_submitted", "Proposal submitted", ["approvers"]],
  ["proposal_approved", "Proposal approved", ["person"]],
  ["proposal_declined", "Proposal declined", ["person"]],
  ["acquisition_available", "Acquisition available", ["person", "operators"]],
  ["acquisition_gave_up", "Acquisition gave up", ["person", "operators"]],
  ["channel_live", "Channel live", ["person", "operators"]],
  ["channel_degraded", "Channel degraded", ["person", "operators"]],
] as const;

const words = (value: string) =>
  value
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");

const compatibleTopics = (audience: Audience) =>
  topics.filter(([, , audiences]) => (audiences as readonly string[]).includes(audience));

const useRefresh = () => {
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: notificationsApi.getNotificationDestinationsListQueryKey() });
};

const TopicChoices = ({
  audience,
  selected,
  onChange,
  prefix,
}: {
  audience: Audience;
  selected: string[];
  onChange: (topics: string[]) => void;
  prefix: string;
}) => (
  <fieldset className="grid gap-2 sm:grid-cols-2">
    <legend className="mb-2 font-medium text-sm">Events</legend>
    {compatibleTopics(audience).map(([value, label]) => (
      <label className="flex items-center gap-2 text-sm" key={value} htmlFor={`${prefix}-${value}`}>
        <Checkbox
          id={`${prefix}-${value}`}
          checked={selected.includes(value)}
          onChange={(event) =>
            onChange(
              event.target.checked ? [...selected, value] : selected.filter((topic) => topic !== value),
            )
          }
        />
        {label}
      </label>
    ))}
  </fieldset>
);

const DestinationRow = ({ destination }: { destination: NotificationDestinationDTO }) => {
  const refresh = useRefresh();
  const [editing, setEditing] = useState(false);
  const [label, setLabel] = useState(destination.label);
  const [audience, setAudience] = useState<Audience>(destination.audience as Audience);
  const [selectedTopics, setSelectedTopics] = useState(destination.topics);
  const [enabled, setEnabled] = useState(destination.enabled);
  const [message, setMessage] = useState("");
  const update = notificationsApi.useNotificationDestinationsUpdate({
    mutation: {
      onSuccess: (response) => {
        if (response.status === 200) setEnabled(response.data.enabled);
        setEditing(false);
        setMessage("Destination updated.");
      },
      onError: () => setMessage("Loomarr could not update this destination."),
    },
  });
  const remove = notificationsApi.useNotificationDestinationsDelete({
    mutation: {
      onSuccess: () => void refresh(),
      onError: () => setMessage("Loomarr could not delete this destination."),
    },
  });
  const test = notificationsApi.useNotificationDestinationsTest({
    mutation: {
      onSuccess: (response) => {
        if (response.status === 202) setMessage(response.data.hint);
        void refresh();
      },
      onError: () => setMessage("Loomarr could not queue the destination test."),
    },
  });

  const save = (nextEnabled = enabled) => {
    if (!label.trim() || selectedTopics.length === 0) return;
    update.mutate({
      id: destination.id,
      data: { label: label.trim(), audience, topics: selectedTopics, enabled: nextEnabled },
    });
  };
  const health = destination.health;

  return (
    <li>
      <Card className="p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="font-medium">{label}</h3>
              <Badge variant={enabled ? "signal" : "neutral"}>{enabled ? "Enabled" : "Draft"}</Badge>
              <Badge variant="neutral">{words(destination.means)}</Badge>
            </div>
            <p className="mt-1 text-muted-foreground text-sm">
              {words(audience)} · {selectedTopics.map(words).join(", ")}
            </p>
            {health ? (
              <div className="mt-2 text-muted-foreground text-xs">
                <p>
                  {health.queuedCount} queued · {health.terminalFailureCount} failed
                </p>
                {health.lastSuccessAt ? (
                  <p>Last accepted {new Date(health.lastSuccessAt).toLocaleString()}</p>
                ) : null}
                {health.lastFailureOutcome ? <p>Last failure: {words(health.lastFailureOutcome)}</p> : null}
              </div>
            ) : null}
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={() => setEditing((value) => !value)}>
              Edit {label}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={!enabled || test.isPending}
              onClick={() => test.mutate({ id: destination.id, data: { requestId: crypto.randomUUID() } })}
            >
              Test {label}
            </Button>
            <Button variant="outline" size="sm" disabled={update.isPending} onClick={() => save(!enabled)}>
              {enabled ? "Disable" : "Enable"} {label}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              disabled={remove.isPending}
              onClick={() => remove.mutate({ id: destination.id })}
            >
              Delete {label}
            </Button>
          </div>
        </div>

        {editing ? (
          <div className="mt-4 grid gap-4 border-border border-t pt-4">
            <div className="grid gap-1.5">
              <Label htmlFor={`edit-label-${destination.id}`}>Edit label</Label>
              <Input
                id={`edit-label-${destination.id}`}
                value={label}
                onChange={(event) => setLabel(event.target.value)}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor={`edit-audience-${destination.id}`}>Edit audience</Label>
              <select
                id={`edit-audience-${destination.id}`}
                className="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
                value={audience}
                onChange={(event) => {
                  const next = event.target.value as Audience;
                  setAudience(next);
                  setSelectedTopics([]);
                }}
              >
                {destination.scope === "installation" ? (
                  <>
                    <option value="operators">Operators</option>
                    <option value="approvers">Approvers</option>
                  </>
                ) : (
                  <option value="person">Me</option>
                )}
              </select>
            </div>
            <TopicChoices
              audience={audience}
              selected={selectedTopics}
              onChange={setSelectedTopics}
              prefix={`edit-${destination.id}`}
            />
            <Button
              className="w-fit"
              disabled={!label.trim() || selectedTopics.length === 0 || update.isPending}
              onClick={() => save()}
            >
              Save {label}
            </Button>
          </div>
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

const NotificationDestinationsPanel = ({ scope }: { scope: DestinationScope }) => {
  const query = notificationsApi.useNotificationDestinationsList();
  const refresh = useRefresh();
  const destinations = useMemo(
    () =>
      query.data?.status === 200 ? query.data.data.destinations.filter((item) => item.scope === scope) : [],
    [query.data, scope],
  );
  const [label, setLabel] = useState("");
  const [provider, setProvider] = useState(scope === "installation" ? "slack" : "email");
  const [audience, setAudience] = useState<Audience>(scope === "installation" ? "operators" : "person");
  const [selectedTopics, setSelectedTopics] = useState<string[]>([]);
  const [message, setMessage] = useState("");
  const create = notificationsApi.useNotificationDestinationsCreate({
    mutation: {
      onSuccess: () => {
        setLabel("");
        setSelectedTopics([]);
        setMessage("Destination draft created. Add its provider details before enabling it.");
        void refresh();
      },
      onError: () => setMessage("Loomarr could not create this destination draft."),
    },
  });
  const providers = scope === "installation" ? installationProviders : personalProviders;

  return (
    <section className="grid gap-4" aria-labelledby={`notification-destinations-${scope}`}>
      <div className="flex gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-signal/10 text-signal">
          <BellRing className="size-4" aria-hidden />
        </span>
        <div>
          <h2 id={`notification-destinations-${scope}`} className="font-semibold text-lg">
            {scope === "installation" ? "Product notification destinations" : "Your notifications"}
          </h2>
          <p className="mt-1 text-muted-foreground text-sm">
            {scope === "installation"
              ? "Route selected product events to shared provider destinations. Credentials stay write-only."
              : "Choose the product events Loomarr may send only to your verified contact or browser."}
          </p>
        </div>
      </div>

      {query.isLoading ? <p className="text-muted-foreground text-sm">Loading destinations…</p> : null}
      {query.isError ? (
        <p className="text-destructive text-sm" role="alert">
          Destinations could not be loaded.
        </p>
      ) : null}
      {!query.isLoading && destinations.length === 0 ? (
        <p className="rounded-lg border border-border border-dashed p-4 text-muted-foreground text-sm">
          No {scope === "installation" ? "shared" : "personal"} product notification destinations yet.
        </p>
      ) : null}
      <ul className="grid gap-3">
        {destinations.map((destination) => (
          <DestinationRow destination={destination} key={destination.id} />
        ))}
      </ul>

      <Card className="grid gap-4 p-4">
        <div className="flex items-center gap-2">
          <Plus className="size-4 text-signal" aria-hidden />
          <h3 className="font-medium">New destination draft</h3>
        </div>
        <p className="text-muted-foreground text-sm">
          Save routing preferences now. A destination stays disabled until its provider details are complete.
        </p>
        <div className="grid gap-4 sm:grid-cols-3">
          <div className="grid gap-1.5">
            <Label htmlFor={`destination-label-${scope}`}>Destination label</Label>
            <Input
              id={`destination-label-${scope}`}
              value={label}
              onChange={(event) => setLabel(event.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor={`destination-provider-${scope}`}>Provider</Label>
            <select
              id={`destination-provider-${scope}`}
              className="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
              value={provider}
              onChange={(event) => setProvider(event.target.value)}
            >
              {providers.map((value) => (
                <option value={value} key={value}>
                  {words(value)}
                </option>
              ))}
            </select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor={`destination-audience-${scope}`}>Audience</Label>
            <select
              id={`destination-audience-${scope}`}
              className="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
              value={audience}
              onChange={(event) => {
                setAudience(event.target.value as Audience);
                setSelectedTopics([]);
              }}
            >
              {scope === "installation" ? (
                <>
                  <option value="operators">Operators</option>
                  <option value="approvers">Approvers</option>
                </>
              ) : (
                <option value="person">Me</option>
              )}
            </select>
          </div>
        </div>
        <TopicChoices
          audience={audience}
          selected={selectedTopics}
          onChange={setSelectedTopics}
          prefix={`new-${scope}`}
        />
        <Button
          className="w-fit"
          disabled={!label.trim() || selectedTopics.length === 0 || create.isPending}
          onClick={() =>
            create.mutate({
              data: {
                means: provider as never,
                label: label.trim(),
                scope,
                audience,
                topics: selectedTopics,
                enabled: false,
              },
            })
          }
        >
          Create destination draft
        </Button>
        {message ? (
          <p className="text-sm" role="status" aria-live="polite">
            {message}
          </p>
        ) : null}
      </Card>
    </section>
  );
};

export { NotificationDestinationsPanel };
