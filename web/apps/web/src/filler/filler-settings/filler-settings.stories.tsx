import type { FillerReadinessDTO } from "@loomarr/api";
import {
  getFillerReadinessQueryKey,
  getFillerResearchStatusQueryKey,
  getListFillerSourcesQueryKey,
} from "@loomarr/api/endpoints/filler";
import { getSettingsListQueryKey } from "@loomarr/api/endpoints/settings";
import { fillerRefinementSettings } from "@loomarr/fixtures";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SettingsEditsProvider } from "@/settings/settings-edits";
import { widthFrame, withRouter } from "@/test/story-utils";
import { FillerSettings } from "./filler-settings";

const readiness: FillerReadinessDTO = {
  ready: true,
  nextAction: "none",
  repairs: { count: 0 },
  fetch: { enabled: true, catalogClips: 48 },
  storage: {
    automatic: true,
    state: "healthy",
    totalBytes: 500 * 1024 ** 3,
    freeBytes: 200 * 1024 ** 3,
    managedBytes: 2 * 1024 ** 3,
    reservedBytes: 0,
    filesystemReservedBytes: 0,
    softBudgetBytes: 20 * 1024 ** 3,
    hardReserveBytes: 10 * 1024 ** 3,
    availableBytes: 18 * 1024 ** 3,
  },
  pipeline: {
    runnable: 0,
    scheduled: 0,
    inProgress: 0,
    needsDecision: 0,
    recoverable: 0,
    ready: 48,
    complete: 0,
    rejected: 0,
    dismissed: 0,
  },
  pool: { clips: 48, breakBody: 38, eligible: 42, untagged: 0, channels: [] },
  acquisitions: [],
};

const withSettings: Decorator = (Story) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(getSettingsListQueryKey(), {
    status: 200,
    data: fillerRefinementSettings,
    headers: new Headers(),
  });
  client.setQueryData(getListFillerSourcesQueryKey(), {
    status: 200,
    data: { sources: [], total: 0 },
    headers: new Headers(),
  });
  client.setQueryData(getFillerReadinessQueryKey(), {
    status: 200,
    data: readiness,
    headers: new Headers(),
  });
  client.setQueryData(getFillerResearchStatusQueryKey(), {
    status: 200,
    data: {
      structuredEnabled: true,
      provider: "none",
      configured: false,
      state: "unconfigured",
      month: "2026-09",
      requestCount: 0,
      requestLimit: 100,
    },
    headers: new Headers(),
  });
  return (
    <QueryClientProvider client={client}>
      <SettingsEditsProvider>
        <Story />
      </SettingsEditsProvider>
    </QueryClientProvider>
  );
};
const meta = {
  title: "Filler/Settings",
  component: FillerSettings,
  decorators: [widthFrame(760), withSettings],
} satisfies Meta<typeof FillerSettings>;
type Story = StoryObj<typeof meta>;

const Downloads: Story = { decorators: [withRouter("/filler")] };
const Storage: Story = {
  args: { section: "storage" },
  decorators: [withRouter("/filler")],
};
const Details: Story = {
  args: { section: "details" },
  decorators: [withRouter("/filler/settings/details")],
  play: async ({ canvas }) => {
    await canvas.findByText(/4 of 4 sources on/);
  },
};

export default meta;
export { Details, Downloads, Storage };
