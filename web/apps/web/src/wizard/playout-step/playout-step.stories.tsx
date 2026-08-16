import type { SettingEntry, SettingsListOutputBody, SetupStatusOutputBody } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { PLAYOUT_INTERNAL, PLAYOUT_TUNARR } from "../steps";
import { PlayoutStep } from "./playout-step";

const jsonResponse = <T,>(body: T) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

const entry = (
  overrides: Partial<SettingEntry> & Pick<SettingEntry, "key" | "group" | "kind">,
): SettingEntry => ({
  advanced: false,
  secret: false,
  set: true,
  provenance: "db",
  doc: `${overrides.key} setup value.`,
  value: "",
  ...overrides,
});

// Both forks need a visual review surface, but this source story deliberately creates no
// baseline: the dashboard work currently owns that shared output. The normal Storybook build
// still proves each fork renders offline from registry-shaped responses.
const withRegistry: Decorator = (Story, context) => {
  const backend = context.args.value === PLAYOUT_TUNARR ? PLAYOUT_TUNARR : PLAYOUT_INTERNAL;
  const publicURL = context.parameters.playout as
    | { value?: string; provenance?: SettingEntry["provenance"] }
    | undefined;
  const settings: SettingsListOutputBody = {
    features: {},
    settings: [
      entry({
        key: "playout.backend",
        group: "playout",
        kind: "enum",
        enum: [PLAYOUT_INTERNAL, PLAYOUT_TUNARR],
        value: backend,
      }),
      entry({
        key: "server.public_url",
        group: "playout",
        kind: "url",
        label: "Loomarr address",
        doc: "Loomarr's own address as your media server can reach it.",
        value: publicURL?.value ?? "http://loomarr:8080",
        provenance: publicURL?.provenance ?? "db",
        envVar: publicURL?.provenance === "env" ? "SERVER_PUBLIC_URL" : undefined,
      }),
      entry({
        key: "tunarr.url",
        group: "connections.tunarr",
        kind: "url",
        label: "Tunarr URL",
        doc: "Base URL of the Tunarr server.",
        value: "http://tunarr:8000",
      }),
    ],
  };
  const status: SetupStatusOutputBody = {
    checks: [{ name: "tunarr", ok: true, hint: "Connection OK" }],
  };

  window.fetch = ((url: string) => {
    if (url.includes("/v1/settings")) return Promise.resolve(jsonResponse(settings));
    if (url.includes("/v1/setup/status")) return Promise.resolve(jsonResponse(status));
    return Promise.resolve(jsonResponse({}));
  }) as typeof fetch;

  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
};

const meta = {
  title: "Wizard/PlayoutStep",
  component: PlayoutStep,
  args: { value: PLAYOUT_INTERNAL },
  decorators: [withRegistry, widthFrame(640)],
} satisfies Meta<typeof PlayoutStep>;

type Story = StoryObj<typeof meta>;

const InternalConfigured: Story = {};
const InternalEmpty: Story = { parameters: { playout: { value: "" } } };
const InternalEnvPinned: Story = {
  parameters: { playout: { value: "http://loomarr:8080", provenance: "env" } },
};
const Tunarr: Story = { args: { value: PLAYOUT_TUNARR } };

export default meta;
export { InternalConfigured, InternalEmpty, InternalEnvPinned, Tunarr };
