import type { FillerResearchStatusDTO, SettingEntry } from "@loomarr/api";
import { getFillerResearchStatusQueryKey } from "@loomarr/api/endpoints/filler";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { ClipDetailsSettings } from "./clip-details-settings";

const entries: SettingEntry[] = [
  ...(
    [
      ["filler.research.wikidata_enabled", "Wikidata"],
      ["filler.research.wikipedia_enabled", "Wikipedia"],
      ["filler.research.archive_enabled", "Archive.org"],
      ["filler.research.loc_enabled", "Library of Congress"],
    ] as const
  ).map(([key, label]) => ({
    key,
    label,
    value: "true",
    kind: "bool" as const,
    presentation: "switch" as const,
    group: "filler",
    owner: "filler.details",
    advanced: true,
    doc: `Search ${label}.`,
    provenance: "default" as const,
    apply: "live" as const,
    secret: false,
    set: true,
  })),
  {
    key: "filler.research.monthly_limit",
    label: "Monthly web searches",
    value: "100",
    kind: "int",
    group: "filler",
    owner: "filler.details",
    advanced: true,
    doc: "The most general-web searches Loomarr may make each month.",
    provenance: "default",
    apply: "live",
    secret: false,
    set: true,
  },
  {
    key: "filler.research.searxng_url",
    label: "SearXNG address",
    value: "",
    kind: "url",
    group: "filler",
    owner: "filler.details",
    advanced: true,
    doc: "The address of a self-hosted SearXNG server.",
    provenance: "default",
    apply: "live",
    secret: false,
    set: false,
  },
];

const withStatus =
  (status: FillerResearchStatusDTO): Decorator =>
  (Story) => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
    client.setQueryData(getFillerResearchStatusQueryKey(), {
      status: 200,
      data: status,
      headers: new Headers(),
    });
    return (
      <QueryClientProvider client={client}>
        <Story />
      </QueryClientProvider>
    );
  };

const base: FillerResearchStatusDTO = {
  structuredEnabled: true,
  provider: "none",
  configured: false,
  state: "unconfigured",
  month: "2026-09",
  requestCount: 0,
  requestLimit: 100,
};

const meta = {
  title: "Filler/Clip details settings",
  component: ClipDetailsSettings,
  args: {
    entries,
    liveValue: (key: string) =>
      key === "filler.research.enabled" || key.endsWith("_enabled") ? "true" : "100",
    setEdit: () => {},
  },
  decorators: [widthFrame(720)],
} satisfies Meta<typeof ClipDetailsSettings>;

export default meta;
type Story = StoryObj<typeof meta>;

const Unconfigured: Story = { decorators: [withStatus(base)] };

const Ready: Story = {
  decorators: [
    withStatus({
      ...base,
      provider: "brave",
      configured: true,
      state: "ready",
      requestCount: 17,
      lastSuccessAt: "2026-09-20T12:00:00Z",
    }),
  ],
};

const Degraded: Story = {
  decorators: [
    withStatus({
      ...base,
      provider: "searxng",
      configured: true,
      state: "degraded",
      requestCount: 4,
      lastFailureAt: "2026-09-20T12:00:00Z",
    }),
  ],
};

const Sources: Story = {
  ...Ready,
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByText("Where Loomarr looks", { exact: true }));
  },
};

const SetupSheet: Story = {
  decorators: [withStatus(base)],
  tags: ["portal"],
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Add web search" }));
  },
};

export { Degraded, Ready, SetupSheet, Sources, Unconfigured };
