import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useMemo } from "react";
import { NotificationDestinationsPanel } from "./notification-destinations-panel";

type State = "empty" | "healthy" | "failed";

const destination = {
  id: "destination-story",
  type: "slack",
  label: "Operations Slack",
  events: ["acquisition_gave_up", "channel_degraded"],
  enabled: true,
  settings: [{ key: "webhookUrl", secretConfigured: true }],
  createdAt: "2026-08-31T18:00:00Z",
  updatedAt: "2026-08-31T18:00:00Z",
};

const storyFetch =
  (state: State): typeof fetch =>
  async (input, init) => {
    const request = input instanceof Request ? input : new Request(input, init);
    if (new URL(request.url).pathname === "/v1/notifications/provider-types") {
      return new Response(
        JSON.stringify({
          providers: [
            {
              type: "slack",
              name: "Slack",
              memberOwned: false,
              events: ["proposal_submitted", "acquisition_gave_up", "channel_degraded"],
              fields: [
                {
                  key: "webhookUrl",
                  label: "Slack webhook URL",
                  kind: "url",
                  required: true,
                  sensitive: true,
                },
              ],
            },
          ],
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    }
    if (new URL(request.url).pathname === "/v1/notifications/providers") {
      const providers =
        state === "empty"
          ? []
          : [
              {
                ...destination,
                health:
                  state === "failed"
                    ? {
                        queuedCount: 2,
                        terminalFailureCount: 1,
                        lastFailureAt: "2026-08-31T20:00:00Z",
                        lastFailureOutcome: "transport_unavailable",
                      }
                    : { queuedCount: 0, terminalFailureCount: 0, lastSuccessAt: "2026-08-31T19:00:00Z" },
              },
            ];
      return new Response(JSON.stringify({ providers }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }
    return new Response(JSON.stringify({ title: "Unstubbed notification story request", status: 500 }), {
      status: 500,
      headers: { "content-type": "application/json" },
    });
  };

const StorySurface = ({ state }: { state: State }) => {
  window.fetch = storyFetch(state);
  const client = useMemo(() => new QueryClient({ defaultOptions: { queries: { retry: false } } }), []);
  return (
    <QueryClientProvider client={client}>
      <div className="min-h-screen bg-background p-6 text-foreground">
        <div className="mx-auto max-w-3xl">
          <NotificationDestinationsPanel />
        </div>
      </div>
    </QueryClientProvider>
  );
};

const meta = {
  title: "Settings/NotificationDestinations",
  component: StorySurface,
  args: { state: "healthy" },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof StorySurface>;

type Story = StoryObj<typeof meta>;
const Healthy: Story = {};
const Failure: Story = { args: { state: "failed" } };
const Empty: Story = { args: { state: "empty" } };

export default meta;
export { Empty, Failure, Healthy };
