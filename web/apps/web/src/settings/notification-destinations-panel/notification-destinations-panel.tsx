import * as notificationsApi from "@loomarr/api/endpoints/notifications";
import { Action, Icon, icons, Stack, Surface, Text } from "@loomarr/design-system";
import { useMemo, useState } from "react";
import { BrowserSelectControl } from "@/client-adapters";
import { ProviderForm } from "./provider-form";
import { ProviderRow } from "./provider-row";
import { words } from "./provider-words";
import { useRefreshProviders } from "./use-refresh-providers";
import { WebPushForm } from "./web-push-form";

const NotificationDestinationsPanel = () => {
  const typesQuery = notificationsApi.useNotificationProviderTypesList();
  const providersQuery = notificationsApi.useNotificationProvidersList();
  const refresh = useRefreshProviders();
  const [adding, setAdding] = useState(false);
  const [selectedType, setSelectedType] = useState("email");
  const [message, setMessage] = useState("");
  const definitions = useMemo(
    () => (typesQuery.data?.status === 200 ? typesQuery.data.data.providers : []),
    [typesQuery.data],
  );
  const providers = providersQuery.data?.status === 200 ? providersQuery.data.data.providers : [];
  // Everything any provider type can be told about, once each: the empty state's answer to
  // "what would I be notified about?".
  const events = useMemo(() => [...new Set(definitions.flatMap((item) => item.events))], [definitions]);
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
      <Stack align="start" direction="row" gap="control" justify="between" wrap>
        <Stack direction="row" gap="control">
          <Stack
            align="center"
            className="bg-signal/10 text-signal size-9 rounded-lg"
            flexShrink={0}
            justify="center"
            render="span"
          >
            <Icon decorative glyph={icons.notifications} size="compact" tone="inherit" />
          </Stack>
          <div>
            <Text as="h2" id="notification-providers-title" strong textRole="headline">
              Notification providers
            </Text>
            <Text as="p" className="mt-1" textRole="compact" tone="secondary">
              Add SMTP, Slack, Discord, or another provider, then choose the events it receives.
            </Text>
          </div>
        </Stack>
        {!adding ? (
          <Action icon="plus" onPress={() => setAdding(true)}>
            Add provider
          </Action>
        ) : null}
      </Stack>

      {providersQuery.isLoading || typesQuery.isLoading ? (
        <Text as="p" textRole="compact" tone="secondary">
          Loading providers…
        </Text>
      ) : null}
      {providersQuery.isError || typesQuery.isError ? (
        <div role="alert">
          <Text as="p" textRole="compact" tone="danger">
            Providers could not be loaded.
          </Text>
        </div>
      ) : null}
      {!providersQuery.isLoading && providers.length === 0 ? (
        <section
          aria-labelledby="notification-events-title"
          className="border-border rounded-lg border border-dashed p-4"
        >
          <Surface backgroundColor="transparent" borderRadius={0} borderWidth={0} gap="inline">
            <Text as="p" id="notification-events-title" textRole="label">
              No notification providers yet
            </Text>
            <Text as="p" textRole="body" tone="muted">
              Once you add one, you choose which of these it tells you about.
            </Text>
            {events.length > 0 ? (
              <Surface backgroundColor="transparent" borderRadius={0} borderWidth={0} gap={4} role="list">
                {events.map((event) => (
                  <Surface
                    backgroundColor="transparent"
                    borderRadius={0}
                    borderWidth={0}
                    key={event}
                    role="listitem"
                  >
                    <Text textRole="body" tone="muted">
                      {words(event)}
                    </Text>
                  </Surface>
                ))}
              </Surface>
            ) : null}
          </Surface>
        </section>
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
          <div className="max-w-sm">
            <BrowserSelectControl
              appearance="field"
              label="Provider"
              onValueChange={setSelectedType}
              options={definitions.map((item) => ({ label: item.name, value: item.type }))}
              value={definition?.type ?? ""}
            />
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
        <div role="status" aria-live="polite">
          <Text as="p" textRole="compact">
            {message}
          </Text>
        </div>
      ) : null}
    </section>
  );
};

export { NotificationDestinationsPanel };
